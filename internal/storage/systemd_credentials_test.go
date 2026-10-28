package storage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/napisani/secret_inject/internal/secret"
)

func TestSystemdCredentialHelperProcess(t *testing.T) {
	marker := slices.Index(os.Args, "--")
	if marker < 0 {
		t.Skip("subprocess helper")
	}
	args := os.Args[marker+1:]
	mode, trace, args := args[0], args[1], args[2:]
	if os.Getenv("PRIVATE_AMBIENT_TEST") != "" {
		os.Exit(3)
	}
	file, err := os.OpenFile(trace, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		os.Exit(4)
	}
	fmt.Fprintln(file, strings.Join(args, " "))
	file.Close()
	if !slices.Contains(args, "--user") || !slices.Contains(args, "--no-ask-password") {
		os.Exit(5)
	}
	switch mode {
	case "fail":
		fmt.Fprintln(os.Stderr, "PRIVATE_NATIVE_ERROR_SENTINEL")
		os.Exit(6)
	case "large":
		os.Stdout.Write(bytes.Repeat([]byte("x"), systemdCipherLimit+1))
		os.Exit(0)
	case "wait":
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(7)
	}
	var name string
	for _, arg := range args {
		if strings.HasPrefix(arg, "--name=") {
			name = strings.TrimPrefix(arg, "--name=")
		}
	}
	if name == "" {
		os.Exit(8)
	}
	if slices.Contains(args, "encrypt") {
		if !slices.Contains(args, "--with-key=host") {
			os.Exit(9)
		}
		data, _ := json.Marshal(struct{ Name, Value string }{name, base64.StdEncoding.EncodeToString(input)})
		fmt.Fprint(os.Stdout, base64.StdEncoding.EncodeToString(data))
	} else {
		if !slices.Contains(args, "--refuse-null") {
			os.Exit(10)
		}
		data, err := base64.StdEncoding.DecodeString(string(input))
		var record struct{ Name, Value string }
		if err != nil || json.Unmarshal(data, &record) != nil || record.Name != name {
			os.Exit(11)
		}
		value, err := base64.StdEncoding.DecodeString(record.Value)
		if err != nil {
			os.Exit(12)
		}
		os.Stdout.Write(value)
	}
	os.Exit(0)
}

func systemdFixture(t *testing.T, mode string) (*systemdCredentials, string) {
	t.Helper()
	root := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
	trace := filepath.Join(root, "trace")
	helper := filepath.Join(root, "helper")
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	script := "#!" + shell + "\nGORACE=atexit_sleep_ms=0 exec " + quote(executable) +
		" -test.run='^TestSystemdCredentialHelperProcess$' -- " + quote(mode) + " " + quote(trace) + " \"$@\"\n"
	if err := os.WriteFile(helper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	hostKey := filepath.Join(root, "synthetic-host-key")
	if err := os.WriteFile(hostKey, []byte("synthetic-only"), 0600); err != nil {
		t.Fatal(err)
	}
	return &systemdCredentials{directory: filepath.Join(root, "records"), executable: helper, hostKey: hostKey}, trace
}

func TestSystemdCredentialsPreparationAndEncryptedTransport(t *testing.T) {
	native, trace := systemdFixture(t, "normal")
	t.Setenv("PRIVATE_AMBIENT_TEST", "MUST_NOT_REACH_NATIVE_HELPER")
	if err := native.prepare(t.Context(), Unattended); !errors.Is(err, ErrInteractionRequired) {
		t.Fatal("unprepared unattended store was accepted")
	}
	if _, err := os.Stat(native.directory); !os.IsNotExist(err) {
		t.Fatal("unattended open created the native directory")
	}
	if _, err := os.Stat(trace); !os.IsNotExist(err) {
		t.Fatal("unattended open initialized the native crypto helper")
	}
	if err := native.prepare(t.Context(), InteractiveSetup); err != nil {
		t.Fatal(err)
	}
	if err := native.Put(t.Context(), "bootstrap-v1/test", []byte("PRIVATE_VALUE_SENTINEL")); err != nil {
		t.Fatal(err)
	}
	name, _ := systemdObjectName("bootstrap-v1/test")
	cipher, err := os.ReadFile(filepath.Join(native.directory, name))
	if err != nil || bytes.Contains(cipher, []byte("PRIVATE_VALUE_SENTINEL")) {
		t.Fatal("transport persisted plaintext")
	}
	before, _ := os.ReadFile(trace)
	keys, err := native.Keys(t.Context())
	after, _ := os.ReadFile(trace)
	if err != nil || !slices.Equal(keys, []string{"bootstrap-v1/test"}) || !bytes.Equal(before, after) {
		t.Fatal("native metadata enumeration decrypted values")
	}
	value, found, err := native.Get(t.Context(), "bootstrap-v1/test")
	if err != nil || !found || string(value) != "PRIVATE_VALUE_SENTINEL" {
		t.Fatal("native roundtrip failed")
	}
	other, _ := systemdObjectName("bootstrap-v1/other")
	if err := os.WriteFile(filepath.Join(native.directory, other), cipher, 0600); err != nil {
		t.Fatal(err)
	}
	if value, _, err := native.Get(t.Context(), "bootstrap-v1/other"); !errors.Is(err, ErrStoreOperation) || value != nil {
		t.Fatal("renamed ciphertext was accepted")
	}
	if err := native.Delete(t.Context(), "bootstrap-v1/test"); err != nil {
		t.Fatal(err)
	}
	if _, found, err := native.Get(t.Context(), "bootstrap-v1/test"); err != nil || found {
		t.Fatal("deleted value survived")
	}
	if err := native.Delete(t.Context(), "bootstrap-v1/test"); err != nil {
		t.Fatal("deletion was not idempotent")
	}
}

func TestSystemdCredentialsBoundaries(t *testing.T) {
	for _, mode := range []string{"fail", "large", "wait"} {
		t.Run(mode, func(t *testing.T) {
			native, _ := systemdFixture(t, mode)
			ctx := t.Context()
			if mode == "wait" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
			}
			_, err := native.crypt(ctx, "encrypt", "synthetic", []byte("SYNTHETIC"))
			if err == nil || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("native failure was ignored or leaked details")
			}
			if mode == "wait" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("native helper ignored cancellation")
			}
		})
	}
}

func TestSystemdCredentialsUseSharedRecordEngine(t *testing.T) {
	native, _ := systemdFixture(t, "normal")
	if err := native.prepare(t.Context(), InteractiveSetup); err != nil {
		t.Fatal(err)
	}
	checkSystemdRecordLifecycle(t, native)
}

func checkSystemdRecordLifecycle(t *testing.T, native *systemdCredentials) {
	t.Helper()
	lock := NewStoreFileLock(filepath.Join(t.TempDir(), "locks"), "synthetic-systemd-store")
	store, err := NewRecordStore(native, lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetCredential(t.Context(), "synthetic-root", "doppler", "SYNTHETIC_BOOTSTRAP_VALUE"); err != nil {
		t.Fatal(err)
	}
	credentials, err := store.AcquireCredentials(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := systemdObjectName("bootstrap-v1/synthetic-root")
	other, _ := systemdObjectName("bootstrap-v1/renamed-root")
	cipher, err := os.ReadFile(filepath.Join(native.directory, original))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(native.directory, other), cipher, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := native.Get(t.Context(), "bootstrap-v1/renamed-root"); !errors.Is(err, ErrStoreOperation) {
		t.Fatal("native credential-name binding accepted renamed ciphertext")
	}
	if err := native.Delete(t.Context(), "bootstrap-v1/renamed-root"); err != nil {
		t.Fatal(err)
	}
	snapshot := secret.New()
	snapshot.Entries["SYNTHETIC_WORKLOAD"] = strings.Repeat("synthetic-value-", 12000)
	scope := strings.Repeat("a", 64)
	if err := store.WriteSnapshot(t.Context(), scope, credentials.Revision, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := native.prepare(t.Context(), Unattended); err != nil {
		t.Fatal(err)
	}
	store, err = NewRecordStore(native, lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	started := time.Now()
	cached, found, err := store.ReadSnapshot(t.Context(), scope, time.Hour)
	t.Logf("synthetic multi-chunk cache read: %s", time.Since(started))
	if err != nil || !found || cached.Secrets.Entries["SYNTHETIC_WORKLOAD"] != snapshot.Entries["SYNTHETIC_WORKLOAD"] {
		t.Fatal("reopened store did not preserve the multi-chunk snapshot")
	}
	if err := store.CleanSnapshots(t.Context()); err != nil {
		t.Fatal(err)
	}
	value, err := store.ReadCredential(t.Context(), "synthetic-root", "doppler")
	if err != nil || value != "SYNTHETIC_BOOTSTRAP_VALUE" {
		t.Fatal("snapshot cleanup damaged a bootstrap credential")
	}
}
