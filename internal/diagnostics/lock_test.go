package diagnostics

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
)

func TestDebugLockHelper(t *testing.T) {
	if os.Getenv("SECRET_INJECT_LOCK_HELPER") != "1" {
		return
	}
	s, err := Open()
	if err != nil {
		os.Exit(3)
	}
	fmt.Fprintln(os.Stdout, "locked")
	io.Copy(io.Discard, os.Stdin)
	if err := s.Finish(); err != nil {
		os.Exit(4)
	}
	os.Exit(0)
}

func TestDebugLockReleasedAfterProcessDeath(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestDebugLockHelper$")
	cmd.Env = append(os.Environ(), "SECRET_INJECT_LOCK_HELPER=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	})
	ready, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || ready != "locked\n" {
		t.Fatal("helper did not acquire lock")
	}
	if s, err := Open(); err == nil {
		s.Finish()
		t.Fatal("cross-process lock was not exclusive")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	s, err := Open()
	if err != nil {
		t.Fatal("process death leaked lock")
	}
	if err := s.Finish(); err != nil {
		t.Fatal(err)
	}
}
