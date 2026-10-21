package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
 "slices"
	"strings"
	"testing"
)

func TestConfigInitUsesCLIOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	code, out, stderr := invoke("config", "init", "--config", path)
	if code != 0 || stderr != "" {
		t.Fatalf("config init: code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(out, "Config written to "+path) {
		t.Fatal("config confirmation bypassed CLI stdout")
	}
}

func TestConfigHelpAndArgumentValidation(t *testing.T) {
	cases := []struct {
		name     string
		command  string
		args     []string
		wantCode int
		wantHelp bool
	}{
		{name: "init help", command: "init", args: []string{"--help"}, wantHelp: true},
		{name: "edit help", command: "edit", args: []string{"--help"}, wantHelp: true},
		{name: "init positional argument", command: "init", args: []string{"unexpected"}, wantCode: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			args := append([]string{"config", tc.command, "--config", path}, tc.args...)
			code, out, stderr := invoke(args...)
			if code != tc.wantCode || out != "" {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out, stderr)
			}
			if tc.wantHelp && !strings.Contains(stderr, "Usage of config "+tc.command) {
				t.Fatal("help output was discarded")
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("help or argument failure modified config")
			}
		})
	}
}

func TestConfigInitReportsOutputFailure(t *testing.T) {
	var stderr strings.Builder
	path := filepath.Join(t.TempDir(), "config.json")
	code := runCLI([]string{"config", "init", "--config", path}, shortWriter{}, &stderr)
	if code != 1 || stderr.Len() == 0 {
		t.Fatal("config command ignored confirmation write failure")
	}
}

func TestConfigEditUsesCLIOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	previousLauncher := editorLauncher
	t.Cleanup(func() { editorLauncher = previousLauncher })
	editorLauncher = func(command string, args []string, stdout, stderr io.Writer) error {
		if command != "example-editor" || args[len(args)-1] != path {
			return errors.New("unexpected editor invocation")
		}
		if _, err := fmt.Fprint(stdout, "editor output"); err != nil {
			return err
		}
		_, err := fmt.Fprint(stderr, "editor diagnostic")
		return err
	}
	code, out, stderr := invoke("config", "edit", "--config", path, "--editor", "example-editor --wait")
	if code != 0 || out != "editor output" || stderr != "editor diagnostic" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, stderr)
	}
	code, out, _ = invoke("config", "edit", "--config", path, "unexpected")
	if code != 1 || out != "" {
		t.Fatal("unexpected editor argument was accepted")
	}
}

func TestCLIOutputFailures(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		failStderr bool
	}{
		{name: "version", args: []string{"--version"}},
		{name: "export help", args: []string{"--help"}, failStderr: true},
		{name: "check help", args: []string{"profile", "check", "--help"}, failStderr: true},
		{name: "config help", args: []string{"config", "init", "--help"}, failStderr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout := io.Writer(shortWriter{})
			stderr := io.Writer(io.Discard)
			if tc.failStderr {
				stdout, stderr = stderr, stdout
			}
			if code := runCLI(tc.args, stdout, stderr); code != 1 {
				t.Fatal("CLI ignored output failure")
			}
		})
	}
}

func TestConfigTemplateIsValidJSON(t *testing.T) {
    t.Parallel()
	data := readConfigTemplate()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(data), &parsed); err != nil {
		t.Fatalf("template is not valid JSON: %v", err)
	}

	if _, ok := parsed["sources"].(map[string]any); !ok {
		t.Fatalf("template missing sources map")
	}
	if _, ok := parsed["storage"].(map[string]any); !ok {
		t.Fatalf("template missing storage map")
	}
	if _, ok := parsed["source_sequence"].([]any); !ok {
		t.Fatalf("template missing source_sequence")
	}
}

func TestSplitCommandLine(t *testing.T) {
    t.Parallel()
    cases := []struct {
        name string
        input string
        expected []string
        wantErr bool
    }{
        {
            name: "simple arguments",
            input: "code -w",
            expected: []string{"code", "-w"},
        },
        {
            name: "quoted executable",
            input: `"Visual Studio Code" --wait`,
            expected: []string{"Visual Studio Code", "--wait"},
        },
        {
            name: "quoted argument",
            input: "vim -c 'set number'",
            expected: []string{"vim", "-c", "set number"},
        },
        {
            name: "unterminated quote",
            input: "vim 'unterminated",
            wantErr: true,
        },
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()
            got, err := splitCommandLine(tc.input)
            if tc.wantErr {
                if err == nil {
                    t.Fatal("expected parsing error")
                }
                return
            }
            if err != nil {
                t.Fatal(err)
            }
            if !slices.Equal(got, tc.expected) {
                t.Fatalf("got %q, want %q", got, tc.expected)
            }
        })
    }
}

func TestInitConfigFileCreatesTemplate(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	if err := initConfigFile(cfgPath, false); err != nil {
		t.Fatalf("initConfigFile failed: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("failed reading config: %v", err)
	}

	if string(data) != configTemplate {
		t.Fatalf("config template mismatch\n got: %s\nwant: %s", string(data), configTemplate)
	}

	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}

	if runtime.GOOS != "windows" {
		perm := info.Mode().Perm()
		if perm != 0o600 {
			t.Fatalf("expected permissions 0600, got %o", perm)
		}
	}

	if err := initConfigFile(cfgPath, false); err == nil {
		t.Fatalf("expected error when file already exists without force")
	}

	if err := initConfigFile(cfgPath, true); err != nil {
		t.Fatalf("expected force overwrite to succeed: %v", err)
	}
}

func TestInitConfigFileForceRestrictsExistingPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file permissions")
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := initConfigFile(path, true); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatal("forced config rewrite retained insecure permissions")
	}
}

func TestEditConfigFileUsesEditorLauncher(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("failed to seed config: %v", err)
	}

	var called bool
	var receivedCmd string
	receivedArgs := []string{}
	previousLauncher := editorLauncher
	t.Cleanup(func() { editorLauncher = previousLauncher })
	editorLauncher = func(cmd string, args []string, stdout, stderr io.Writer) error {
		called = true
		receivedCmd = cmd
		if len(args) == 0 || args[len(args)-1] != cfgPath {
			return errors.New("unexpected path")
		}
		receivedArgs = append(receivedArgs, args[:len(args)-1]...)
		return nil
	}
	if err := editConfigFile(
		cfgPath,
		"nano --wait",
		io.Discard,
		io.Discard,
	); err != nil {
		t.Fatalf("editConfigFile failed: %v", err)
	}

	if !called {
		t.Fatalf("editor launcher was not invoked")
	}
	if receivedCmd != "nano" {
		t.Fatalf("expected command 'nano', got %q", receivedCmd)
	}
	if len(receivedArgs) != 1 || receivedArgs[0] != "--wait" {
		t.Fatalf("unexpected args: %v", receivedArgs)
	}
}

func TestEditConfigFileRequiresExistingConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "missing.json")

	err := editConfigFile(
		cfgPath,
		"nano",
		io.Discard,
		io.Discard,
	)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected missing file error, got %v", err)
	}
}
