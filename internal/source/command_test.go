package source

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestProviderCommandHelper(t *testing.T) {
	if os.Getenv("SECRET_INJECT_HELPER") != "1" {
		return
	}
	fmt.Fprintln(os.Stdout, "synthetic-secret-output")
	fmt.Fprintln(os.Stderr, "synthetic-secret-stderr")
	if os.Getenv("SECRET_INJECT_TIMEOUT") == "1" {
		time.Sleep(time.Second)
	}
	os.Exit(7)
}

func TestProviderFailuresDoNotExposeOutputOrArguments(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "SECRET_INJECT_HELPER=1")
	_, err = defaultRunCLICommand(exe, env, "-test.run=TestProviderCommandHelper", "--", "synthetic-secret-reference")
	if err == nil {
		t.Fatal("expected provider failure")
	}
	for _, value := range []string{"synthetic-secret-output", "synthetic-secret-stderr", "synthetic-secret-reference", exe} {
		if strings.Contains(err.Error(), value) {
			t.Fatal("provider error disclosed command data")
		}
	}
	original := commandTimeout
	commandTimeout = 20 * time.Millisecond
	defer func() { commandTimeout = original }()
	_, err = defaultRunCLICommand(exe, append(env, "SECRET_INJECT_TIMEOUT=1"), "-test.run=TestProviderCommandHelper")
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatal("timeout not classified")
	}
}
