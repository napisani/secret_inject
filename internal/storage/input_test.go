package storage

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestPasswordPromptWriteFailureStopsInput(t *testing.T) {
	input, inputWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := inputWriter.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := input.Close(); err != nil {
			t.Error(err)
		}
	})
	output, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	previousIn, previousErr := os.Stdin, os.Stderr
	t.Cleanup(func() { os.Stdin, os.Stderr = previousIn, previousErr })
	os.Stdin, os.Stderr = input, output
	if _, err := getPasswordStdin("Password"); !errors.Is(err, os.ErrClosed) {
		t.Fatal("prompt write failure was ignored before reading input")
	}
}

func TestPasswordPromptDoesNotContaminateStdout(t *testing.T) {
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	writer.Close()
	defer input.Close()
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldIn, oldOut, oldErr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = input, outW, errW
	defer func() { os.Stdin, os.Stdout, os.Stderr = oldIn, oldOut, oldErr }()
	_, readErr := getPasswordStdin("Password")
	outW.Close()
	errW.Close()
	out, _ := io.ReadAll(outR)
	stderr, _ := io.ReadAll(errR)
	outR.Close()
	errR.Close()
	if readErr == nil {
		t.Fatal("pipe unexpectedly a terminal")
	}
	if len(out) != 0 || !strings.Contains(string(stderr), "Password:") {
		t.Fatalf("prompt routing: stdout=%q stderr=%q", out, stderr)
	}
}
