package source

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	name := filepath.Base(os.Args[0])
	if os.Getenv("SECRET_INJECT_PROVIDER_HELPER") == "1" && (name == "doppler" || name == "op" || name == "bws") {
		providerProcessFixture(name)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func providerProcessFixture(name string) {
	fail := func() { os.Exit(19) }
	args := os.Args[1:]
	probe := false
	for _, arg := range args {
		if arg == "--version" {
			probe = true
		}
	}
	if probe {
		for _, key := range []string{"DOPPLER_TOKEN", "OP_SERVICE_ACCOUNT_TOKEN", "OP_CONNECT_TOKEN", "BWS_ACCESS_TOKEN"} {
			if os.Getenv(key) != "" {
				fail()
			}
		}
	}
	for _, key := range []string{"OP_CONNECT_HOST", "OP_SESSION_work", "BWS_SERVER_URL"} {
		if os.Getenv(key) != "" {
			fail()
		}
	}
	var directory string
	switch name {
	case "doppler":
		for index, arg := range args {
			if arg == "--config-dir" && index+1 < len(args) {
				directory = args[index+1]
			}
		}
		data, err := os.ReadFile(filepath.Join(directory, ".doppler.yaml"))
		if err != nil || string(data) != "{}\n" {
			fail()
		}
	case "op":
		for index, arg := range args {
			if arg == "--config" && index+1 < len(args) {
				directory = args[index+1]
			}
		}
		if directory == "" || os.Getenv("OP_BIOMETRIC_UNLOCK_ENABLED") != "false" || os.Getenv("OP_CACHE") != "false" {
			fail()
		}
	case "bws":
		directory = filepath.Dir(os.Getenv("BWS_CONFIG_FILE"))
		data, err := os.ReadFile(os.Getenv("BWS_CONFIG_FILE"))
		if err != nil || !strings.Contains(string(data), `state_opt_out = "true"`) || !strings.Contains(string(data), `server_base = "https://fixture.invalid"`) {
			fail()
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		fail()
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil || strings.Contains(string(data), "PRIVATE_SENTINEL") {
			fail()
		}
	}
	if probe {
		if value, ok := os.LookupEnv("SECRET_INJECT_FIXTURE_VERSION"); ok {
			fmt.Print(value)
		} else {
			fmt.Println(map[string]string{"doppler": "v3.75.1", "op": "2.30.0", "bws": "bws 1.0.0"}[name])
		}
		return
	}
	switch name {
	case "doppler":
		if os.Getenv("DOPPLER_TOKEN") != "ROOT_PRIVATE_SENTINEL" || os.Getenv("OP_SERVICE_ACCOUNT_TOKEN") != "" || os.Getenv("BWS_ACCESS_TOKEN") != "" {
			fail()
		}
		if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"OP_SERVICE_ACCOUNT_TOKEN": map[string]string{"computed": "DERIVED_PRIVATE_SENTINEL"}, "APP_KEY": map[string]string{"computed": "APP_VALUE"}}); err != nil {
			fail()
		}
	case "op":
		if os.Getenv("OP_SERVICE_ACCOUNT_TOKEN") != "DERIVED_PRIVATE_SENTINEL" || os.Getenv("DOPPLER_TOKEN") != "" || os.Getenv("BWS_ACCESS_TOKEN") != "" {
			fail()
		}
		fmt.Println("WORKLOAD_VALUE")
	case "bws":
		if os.Getenv("BWS_ACCESS_TOKEN") != "BWS_PRIVATE_SENTINEL" || os.Getenv("DOPPLER_TOKEN") != "" || os.Getenv("OP_SERVICE_ACCOUNT_TOKEN") != "" {
			fail()
		}
		fmt.Println(`{"id":"fixture-id","key":"BW_KEY","value":"BW_VALUE"}`)
	}
}

func privateProviderProcessFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	tmp := filepath.Join(root, "tmp")
	for _, directory := range []string{bin, tmp} {
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"doppler", "op", "bws"} {
		in, err := os.Open(exe)
		if err != nil {
			t.Fatal(err)
		}
		out, err := os.OpenFile(filepath.Join(bin, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
		if err != nil {
			in.Close()
			t.Fatal(err)
		}
		_, copyErr := io.Copy(out, in)
		inErr, outErr := in.Close(), out.Close()
		if copyErr != nil || inErr != nil || outErr != nil {
			t.Fatal("provider fixture copy failed")
		}
	}
	for name, value := range map[string]string{"PATH": bin, "TMPDIR": tmp, "TMP": tmp, "TEMP": tmp, "SECRET_INJECT_PROVIDER_HELPER": "1", "DOPPLER_TOKEN": "AMBIENT_PRIVATE_SENTINEL", "OP_SERVICE_ACCOUNT_TOKEN": "AMBIENT_PRIVATE_SENTINEL", "OP_CONNECT_TOKEN": "AMBIENT_PRIVATE_SENTINEL", "OP_CONNECT_HOST": "https://ambient.invalid", "OP_SESSION_work": "AMBIENT_PRIVATE_SENTINEL", "BWS_ACCESS_TOKEN": "AMBIENT_PRIVATE_SENTINEL", "BWS_SERVER_URL": "https://fixture.invalid"} {
		t.Setenv(name, value)
	}
	return tmp
}

func TestPrivateProviderVersionOutputValidation(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		output   string
		valid    bool
	}{
		{name: "Doppler", provider: "doppler", output: "v3.75.1\n", valid: true},
		{name: "1Password", provider: "onepassword", output: "2.30.0\n", valid: true},
		{name: "Bitwarden", provider: "bitwarden", output: "bws 1.0.0\n", valid: true},
		{name: "prerelease", provider: "doppler", output: "v4.0.0-beta.1+build.9", valid: true},
		{name: "empty", provider: "doppler"},
		{name: "malformed", provider: "doppler", output: "PRIVATE_VERSION_SENTINEL"},
		{name: "multiline", provider: "doppler", output: "v3.75.1\nPRIVATE_VERSION_SENTINEL"},
		{name: "wrong provider prefix", provider: "onepassword", output: "bws 1.0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := privateProviderProcessFixture(t)
			t.Setenv("SECRET_INJECT_FIXTURE_VERSION", tt.output)
			private, err := newProviderContext(tt.provider)
			if err != nil {
				t.Fatal(err)
			}
			version, probeErr := private.version(t.Context())
			if err := os.RemoveAll(private.directory); err != nil {
				t.Fatal(err)
			}
			if tt.valid {
				if probeErr != nil || version != strings.TrimSpace(tt.output) {
					t.Fatal("well-formed private version probe failed")
				}
			} else if !errors.Is(probeErr, ErrProviderVersion) || version != "" {
				t.Fatal("invalid version probe was accepted")
			}
			if probeErr != nil && strings.Contains(probeErr.Error(), "SENTINEL") {
				t.Fatal("version output entered an error")
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatal("version fixture retained a private configuration")
			}
		})
	}
}

func TestPrivateProviderProcessesIsolateAuthAndCleanConfigurations(t *testing.T) {
	tmp := privateProviderProcessFixture(t)
	configs := map[string]json.RawMessage{
		"doppler":     json.RawMessage(`{"project":"fixture","env":"fixture","credential":{"kind":"stored","name":"root"}}`),
		"onepassword": json.RawMessage(`{"secrets":{"WORKLOAD":"op://fixture/item/field"},"credential":{"kind":"from_source","source":"doppler","key":"OP_SERVICE_ACCOUNT_TOKEN"}}`),
		"bitwarden":   json.RawMessage(`{"secrets":{"BW_KEY":"fixture-id"},"credential":{"kind":"stored","name":"bw-root"}}`),
	}
	roots := func(name, provider string) (string, error) {
		if provider == "bitwarden" {
			return "BWS_PRIVATE_SENTINEL", nil
		}
		return "ROOT_PRIVATE_SENTINEL", nil
	}
	workload, err := ResolvePrivate(t.Context(), configs, []string{"doppler", "onepassword", "bitwarden"}, roots, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(workload.Entries) != 3 || workload.Entries["APP_KEY"] != "APP_VALUE" || workload.Entries["WORKLOAD"] != "WORKLOAD_VALUE" || workload.Entries["BW_KEY"] != "BW_VALUE" {
		t.Fatal("private provider contexts changed workload projection")
	}
	entries, err := os.ReadDir(tmp)
	if err != nil || len(entries) != 0 {
		t.Fatal("provider context was not cleaned")
	}
}
