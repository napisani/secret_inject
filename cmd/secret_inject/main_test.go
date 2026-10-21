package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	if os.Getenv("SECRET_INJECT_FAKE_PROVIDER") == "1" && (name == "doppler" || name == "op") {
		if os.Getenv("SI_PROVIDER_FAIL") == "1" {
			fmt.Fprintln(os.Stderr, "RAW_SECRET_SENTINEL")
			os.Exit(9)
		}
		if name == "op" {
			if os.Getenv("OP_SERVICE_ACCOUNT_TOKEN") != "BOOTSTRAP_SENTINEL" {
				os.Exit(8)
			}
			fmt.Println("FETCHED_SENTINEL")
			os.Exit(0)
		}
		json.NewEncoder(os.Stdout).Encode(map[string]any{
			"OP_SERVICE_ACCOUNT_TOKEN": map[string]string{"computed": "BOOTSTRAP_SENTINEL"},
			"API_KEY":                  map[string]string{"computed": "DISCARDED_SENTINEL"},
			"SERVICE_URL":              map[string]string{"computed": "https://example.test"},
		})
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func commandFixture(t *testing.T, profiles bool) (string, string) {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"home", "cache", "state", "tmp", "bin"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for key, value := range map[string]string{"HOME": "home", "USERPROFILE": "home", "LOCALAPPDATA": "cache", "XDG_CACHE_HOME": "cache", "XDG_STATE_HOME": "state", "TMPDIR": "tmp", "TEMP": "tmp", "TMP": "tmp"} {
		t.Setenv(key, filepath.Join(root, value))
	}
	t.Setenv("SECRET_INJECT_FAKE_PROVIDER", "1")
	t.Setenv("SI_PROVIDER_FAIL", "")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"doppler", "op"} {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		src, err := os.Open(exe)
		if err != nil {
			t.Fatal(err)
		}
		dst, err := os.OpenFile(filepath.Join(root, "bin", name), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0700)
		if err != nil {
			t.Fatal(err)
		}
		_, copyErr := io.Copy(dst, src)
		src.Close()
		closeErr := dst.Close()
		if copyErr != nil || closeErr != nil {
			t.Fatal("cannot create fake provider")
		}
	}
	t.Setenv("PATH", filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	cfg := map[string]any{
		"storage":         map[string]string{"type": "file"},
		"source_sequence": []string{"doppler", "onepassword"},
		"sources":         map[string]any{"doppler": map[string]string{"project": "test", "env": "dev"}, "onepassword": map[string]any{"secrets": map[string]string{"SERVICE_TOKEN": "op://synthetic-reference"}}},
	}
	if profiles {
		cfg["profiles"] = map[string]any{
			"shell": map[string]any{"exclude": []string{"OP_SERVICE_ACCOUNT_TOKEN"}},
			"task":  map[string]any{"include": []string{"SERVICE_*", "API_KEY", "MISSING_*"}, "rename": map[string]string{"SERVICE_TOKEN": "API_KEY"}, "overwrite": false},
		}
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path, root
}

func invoke(args ...string) (int, string, string) {
	var out, stderr bytes.Buffer
	code := runCLI(args, &out, &stderr)
	return code, out.String(), stderr.String()
}

func TestCommandValidationAndWriterFailures(t *testing.T) {
	path, _ := commandFixture(t, true)
	for _, args := range [][]string{
		{"--config", path, "--profile", "unknown"},
		{"--config", path, "--profile", ""},
		{"--config", path, "--clean", "--profile", "task"},
		{"--config", path, "--output", "unsafe"},
		{"profile", "check", "--config", path},
		{"profile", "check", "--config", path, "--profile", "task", "--output", "json"},
	} {
		code, out, _ := invoke(args...)
		if code != 1 || out != "" {
			t.Fatalf("invalid flags produced result: %v", args)
		}
	}
	cache, _ := os.UserCacheDir()
	if _, err := os.Stat(filepath.Join(cache, "secret_inject")); !os.IsNotExist(err) {
		t.Fatal("validation touched storage")
	}
	var stderr bytes.Buffer
	if code := runCLI([]string{"--config", path, "--profile", "task"}, shortWriter{}, &stderr); code != 1 || !strings.Contains(stderr.String(), "write failed") {
		t.Fatal("stdout short write was ignored")
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return 0, nil }

func TestSourceChangesAndExpiryNeverReuseStaleCache(t *testing.T) {
	path, _ := commandFixture(t, true)
	if code, _, err := invoke("--config", path, "--profile", "task"); code != 0 {
		t.Fatal(err)
	}
	t.Setenv("SI_PROVIDER_FAIL", "1")
	if code, _, err := invoke("--config", path, "--profile", "shell"); code != 0 {
		t.Fatal("profiles did not share full cache:", err)
	}
	code, out, _ := invoke("--config", path, "--profile", "task", "--ttl", "0s")
	if code != 1 || out != "" {
		t.Fatal("expired cache silently reused")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(data, []byte(`"project":"test"`), []byte(`"project":"other"`), 1)
	if bytes.Equal(data, changed) {
		t.Fatal("fixture source did not change")
	}
	if err := os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	code, out, _ = invoke("--config", path, "--profile", "task")
	if code != 1 || out != "" {
		t.Fatal("different sources reused another cache")
	}
}

func TestCheckCleanReportAndUnavailableDebug(t *testing.T) {
	path, root := commandFixture(t, true)
	code, out, stderr := invoke("profile", "check", "--config", path, "--profile", "shell")
	if code != 0 || stderr != "" || out != "{\"profile\":\"shell\",\"missing_includes\":[]}\n" {
		t.Fatalf("exclude report: %d %q %q", code, out, stderr)
	}
	t.Setenv("XDG_STATE_HOME", "relative")
	code, out, _ = invoke("--config", path, "--profile", "task", "--debug")
	if code != 1 || out != "" {
		t.Fatal("unavailable requested debug did not stop output")
	}
	if _, err := os.Stat(filepath.Join(root, "state", "secret_inject")); !os.IsNotExist(err) {
		t.Fatal("invalid debug path created logs")
	}
}

func TestProfileCheckHelpUsesCommandName(t *testing.T) {
	code, out, stderr := invoke("profile", "check", "--help")
	if code != 0 || out != "" {
		t.Fatalf("help code=%d stdout=%q", code, out)
	}
	if !strings.Contains(stderr, "Usage of secret_inject profile check:") {
		t.Fatal("check help identifies the wrong command")
	}
}

func TestProfileSelectionRequiredBeforeStorage(t *testing.T) {
	path, _ := commandFixture(t, true)
	code, out, stderr := invoke("--config", path)
	if code != 1 || out != "" || !strings.Contains(stderr, "--profile") {
		t.Fatalf("code=%d out=%q stderr=%q", code, out, stderr)
	}
	cache, _ := os.UserCacheDir()
	if _, err := os.Stat(filepath.Join(cache, "secret_inject")); !os.IsNotExist(err) {
		t.Fatal("invalid profile selection touched cache")
	}
}

func TestLegacyAndProfiledCommandFlows(t *testing.T) {
	path, root := commandFixture(t, false)
	code, out, stderr := invoke("--config", path, "--output", "json")
	if code != 0 || stderr != "" || !strings.Contains(out, "BOOTSTRAP_SENTINEL") {
		t.Fatalf("legacy export failed: %d %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "state", "secret_inject")); !os.IsNotExist(err) {
		t.Fatal("non-debug invocation created logs")
	}
	path, root = commandFixture(t, true)
	code, out, stderr = invoke("--config", path, "--profile", "task", "--output", "json", "--debug")
	if code != 0 || stderr != "" {
		t.Fatalf("profile export: %d %q", code, stderr)
	}
	var projected map[string]string
	if err := json.Unmarshal([]byte(out), &projected); err != nil {
		t.Fatal(err)
	}
	if projected["API_KEY"] != "FETCHED_SENTINEL" || len(projected) != 2 {
		t.Fatal("projection incorrect")
	}
	log, err := os.ReadFile(filepath.Join(root, "state", "secret_inject", "debug.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "rename-collision") || !strings.Contains(string(log), "MISSING_*") {
		t.Fatal("missing diagnostic events")
	}
	for _, sentinel := range []string{"BOOTSTRAP_SENTINEL", "FETCHED_SENTINEL", "DISCARDED_SENTINEL", "op://synthetic-reference"} {
		if strings.Contains(string(log), sentinel) {
			t.Fatal("debug leaked value or reference")
		}
	}
	t.Setenv("SI_PROVIDER_FAIL", "1")
	code, out, stderr = invoke("profile", "check", "--config", path, "--profile", "task")
	if code != 2 || stderr != "" || out != "{\"profile\":\"task\",\"missing_includes\":[\"MISSING_*\"]}\n" {
		t.Fatalf("cached check: %d %q %q", code, out, stderr)
	}
	code, out, stderr = invoke("profile", "check", "--config", path, "--profile", "task", "--force", "--debug")
	if code != 1 || out != "" || strings.Contains(stderr, "RAW_SECRET_SENTINEL") {
		t.Fatalf("unsafe provider failure: %d %q %q", code, out, stderr)
	}
	log, err = os.ReadFile(filepath.Join(root, "state", "secret_inject", "debug.log"))
	if err != nil || strings.Contains(string(log), "RAW_SECRET_SENTINEL") || !strings.Contains(string(log), "provider-failed") {
		t.Fatal("unsafe or missing provider-failure diagnostic")
	}
	code, out, stderr = invoke("--config", path, "--clean")
	if code != 0 || stderr != "" {
		t.Fatalf("cleanup: %d %q", code, stderr)
	}
	code, out, _ = invoke("--config", path, "--profile", "task", "--output", "json")
	if code != 1 || out != "" {
		t.Fatal("cleanup failed to remove scoped snapshot")
	}
}
