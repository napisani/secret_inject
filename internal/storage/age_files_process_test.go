package storage

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/napisani/secret_inject/internal/secret"
)

func TestAgeFilesHelperProcess(t *testing.T) {
	marker := slices.Index(os.Args, "--")
	if marker < 0 {
		t.Skip("subprocess helper")
	}
	args := os.Args[marker+1:]
	if len(args) != 2 {
		t.Fatal("invalid synthetic helper arguments")
	}
	policy := Unattended
	if args[1] == "write" {
		policy = InteractiveSetup
	}
	native, err := openAgeFiles(t.Context(), filepath.Join(args[0], "age"), policy)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewRecordStore(native, NewStoreFileLock(filepath.Join(args[0], "locks"), "age-process-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	scope := strings.Repeat("f", 64)
	if args[1] == "write" {
		if err := store.SetCredential(t.Context(), "synthetic-root", "doppler", "SYNTHETIC_ROOT"); err != nil {
			t.Fatal(err)
		}
		credentials, err := store.AcquireCredentials(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		snapshot := secret.New()
		snapshot.Entries["SYNTHETIC_WORKLOAD"] = "SYNTHETIC_VALUE"
		if err := store.WriteSnapshot(t.Context(), scope, credentials.Revision, snapshot); err != nil {
			t.Fatal(err)
		}
		return
	}
	cached, found, err := store.ReadSnapshot(t.Context(), scope, time.Hour)
	if err != nil || !found || cached.Secrets.Entries["SYNTHETIC_WORKLOAD"] != "SYNTHETIC_VALUE" {
		t.Fatal("independent executable failed to read the synthetic cache")
	}
}

func TestAgeFilesPersistAcrossExecutablesWithoutProviderPath(t *testing.T) {
	t.Parallel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	run := func(executable, mode string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, executable, "-test.run=^TestAgeFilesHelperProcess$", "--", root, mode)
		command.Env = []string{"PATH=", "GORACE=atexit_sleep_ms=0"}
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("synthetic helper failed: %v\n%s", err, output)
		}
	}
	run(executable, "write")
	copyPath := filepath.Join(t.TempDir(), "independent-age-reader")
	source, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := os.OpenFile(copyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		source.Close()
		t.Fatal(err)
	}
	_, copyErr := io.Copy(destination, source)
	sourceErr, destinationErr := source.Close(), destination.Close()
	if copyErr != nil || sourceErr != nil || destinationErr != nil {
		t.Fatal("failed to copy the synthetic test executable")
	}
	if runtime.GOOS == "darwin" {
		command := exec.CommandContext(t.Context(), "/usr/bin/codesign",
			"--force", "--sign", "-", "--identifier", "secret-inject-age-test-reader", copyPath)
		if err := command.Run(); err != nil {
			t.Fatal("failed to ad-hoc sign the independent test executable")
		}
	}
	run(copyPath, "read")
}
