package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestConfigTemplateIsValidJSON(t *testing.T) {
	data := readConfigTemplate()
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(data), &parsed); err != nil {
		t.Fatalf("template is not valid JSON: %v", err)
	}

	if _, ok := parsed["sources"].(map[string]interface{}); !ok {
		t.Fatalf("template missing sources map")
	}
	if _, ok := parsed["storage"].(map[string]interface{}); !ok {
		t.Fatalf("template missing storage map")
	}
	if _, ok := parsed["source_sequence"].([]interface{}); !ok {
		t.Fatalf("template missing source_sequence")
	}
}

func TestSplitCommandLine(t *testing.T) {
	cases := []struct {
		input    string
		expected []string
	}{
		{"code -w", []string{"code", "-w"}},
		{"\"Visual Studio Code\" --wait", []string{"Visual Studio Code", "--wait"}},
		{"vim -c 'set number'", []string{"vim", "-c", "set number"}},
	}

	for _, tc := range cases {
		got, err := splitCommandLine(tc.input)
		if err != nil {
			t.Fatalf("splitCommandLine(%q) returned error: %v", tc.input, err)
		}
		if len(got) != len(tc.expected) {
			t.Fatalf("splitCommandLine(%q) len = %d, want %d", tc.input, len(got), len(tc.expected))
		}
		for i := range tc.expected {
			if got[i] != tc.expected[i] {
				t.Fatalf("splitCommandLine(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.expected[i])
			}
		}
	}

	if _, err := splitCommandLine("vim 'unterminated"); err == nil {
		t.Fatalf("expected error for unterminated quote")
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

func TestEditConfigFileUsesEditorLauncher(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("failed to seed config: %v", err)
	}

	var called bool
	var receivedCmd string
	var receivedArgs []string
	editorLauncher = func(cmd string, args []string, path string) error {
		called = true
		receivedCmd = cmd
		receivedArgs = append([]string(nil), args...)
		if path != cfgPath {
			return errors.New("unexpected path")
		}
		return nil
	}
	t.Cleanup(func() { editorLauncher = defaultEditorLauncher })

	if err := editConfigFile(cfgPath, "nano --wait"); err != nil {
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

	err := editConfigFile(cfgPath, "nano")
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected missing file error, got %v", err)
	}
}
