package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

func TestRunUsageRequiresExplicitProfileAndLiteralSeparator(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"missing separator", []string{"run", "--profile", "task", "target"}},
		{"missing target", []string{"run", "--profile", "task", "--"}},
		{"missing profile", []string{"run", "--", "target"}},
		{"removed inheritance flag", []string{"run", "--inherit-all", "--profile", "task", "--", "target"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, stderr := invoke(tt.args...)
			if code != 2 || out != "" {
				t.Fatal("invalid run invocation accepted")
			}
			assertJSONResponse(t, stderr, `{"schema_version":1,"error":{"code":"usage","message":"invalid invocation; use --help for usage"}}`)
		})
	}
}

func TestRunMissingTargetDoesNotResolveRemoteSecrets(t *testing.T) {
	path, _ := commandFixture(t, true)
	previous := fetchRemoteProvider
	fetchRemoteProvider = nil
	t.Cleanup(func() { fetchRemoteProvider = previous })
	code, out, stderr := invoke("run", "--config", path, "--profile", "task", "--", "MISSING_TARGET_SENTINEL")
	if code != 127 || out != "" || strings.Contains(stderr, "SENTINEL") || !strings.Contains(stderr, "executable_not_found") {
		t.Fatalf("unsafe target lookup failure code=%d stderr=%q", code, stderr)
	}
}

func TestRunExecFailureNeverFallsBackToShell(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	if err := os.WriteFile(path, []byte(`{"profiles":{"task":{"exclude":[],"remote_sources":false}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	previous := replaceProcess
	t.Cleanup(func() { replaceProcess = previous })
	calls := 0
	replaceProcess = func(string, []string, []string) error { calls++; return syscall.ENOEXEC }
	code, out, stderr := invoke("run", "--config", path, "--profile", "task", "--", binary)
	if code != 126 || out != "" || calls != 1 || !strings.Contains(stderr, "executable_not_executable") {
		t.Fatal("exec-format failure used fallback execution")
	}
}

func TestRunRealProcessPreservesStreamsInputAndExitStatus(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	if err := os.WriteFile(path, []byte(`{"profiles":{"task":{"exclude":[],"remote_sources":false}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binary, "run", "--config", path, "--profile", "task", "--", binary, "--child-fixture")
	command.Env = append(os.Environ(), "SECRET_INJECT_TEST_RUN=1")
	command.Stdin = strings.NewReader("TARGET_INPUT_SENTINEL")
	output, err := command.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 37 || !strings.Contains(string(output), "TARGET_INPUT_SENTINEL") || !strings.Contains(string(output), "TARGET_STDERR_SENTINEL") {
		t.Fatalf("target streams/status changed: %q %v", output, err)
	}
}

func TestRunInheritsAndOverlaysWithoutConsumingTargetArguments(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("INHERITED", "parent")
	t.Setenv("REPLACED", "parent")
	t.Setenv("EXCLUDED", "still-parent")
	path := filepath.Join(root, "config.json")
	if err := os.WriteFile(path, []byte(`{"profiles":{"task":{"include":["REPLACED","MISSING"],"overwrite":false,"remote_sources":false,"env_files":["*.env"]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app.env"), []byte("REPLACED=selected\nEXCLUDED=not-selected\n"), 0600); err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	previous := replaceProcess
	t.Cleanup(func() { replaceProcess = previous })
	called := false
	replaceProcess = func(target string, argv, env []string) error {
		called = true
		if target != binary || !reflect.DeepEqual(argv, []string{binary, "--literal-target-flag", "ARGUMENT_SENTINEL"}) {
			t.Fatal("target argv changed")
		}
		values := map[string]string{}
		for _, entry := range env {
			key, value, _ := strings.Cut(entry, "=")
			values[key] = value
		}
		if values["INHERITED"] != "parent" || values["REPLACED"] != "selected" || values["EXCLUDED"] != "still-parent" {
			t.Fatal("run inheritance/projection policy changed")
		}
		return nil
	}
	code, out, stderr := invoke("run", "--config", path, "--profile", "task", "--", binary, "--literal-target-flag", "ARGUMENT_SENTINEL")
	if code != 0 || out != "" || stderr != "" || !called {
		t.Fatalf("run code=%d stderr=%q", code, stderr)
	}
}
