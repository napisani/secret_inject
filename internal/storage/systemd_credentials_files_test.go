package storage

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSystemdCredentialsRejectUnsafeFilesAndMissingHostKey(t *testing.T) {
	native, trace := systemdFixture(t, "normal")
	if err := native.prepare(t.Context(), InteractiveSetup); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "unrelated")
	if err := os.WriteFile(target, []byte("MUST_REMAIN_UNTOUCHED"), 0600); err != nil {
		t.Fatal(err)
	}
	name, _ := systemdObjectName("bootstrap-v1/link")
	if err := os.Symlink(target, filepath.Join(native.directory, name)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := native.Get(t.Context(), "bootstrap-v1/link"); !errors.Is(err, ErrStoreOperation) {
		t.Fatal("native read followed a symlink")
	}
	if err := native.Delete(t.Context(), "bootstrap-v1/link"); !errors.Is(err, ErrStoreOperation) {
		t.Fatal("native delete accepted a symlink")
	}
	if _, err := native.Keys(t.Context()); !errors.Is(err, ErrStoreOperation) {
		t.Fatal("native enumeration accepted a symlink")
	}
	value, err := os.ReadFile(target)
	if err != nil || string(value) != "MUST_REMAIN_UNTOUCHED" {
		t.Fatal("unrelated file changed")
	}
	if err := native.Put(t.Context(), "too-large", make([]byte, systemdValueLimit+1)); !errors.Is(err, ErrStoreOperation) {
		t.Fatal("oversized plaintext was accepted")
	}
	before, _ := os.ReadFile(trace)
	if err := os.Remove(native.hostKey); err != nil {
		t.Fatal(err)
	}
	if err := native.prepare(t.Context(), Unattended); !errors.Is(err, ErrInteractionRequired) {
		t.Fatal("unattended preparation accepted a missing host key")
	}
	if err := native.Put(t.Context(), "must-not-initialize", []byte("SYNTHETIC")); !errors.Is(err, ErrInteractionRequired) {
		t.Fatal("ordinary writes attempted to initialize a host key")
	}
	after, _ := os.ReadFile(trace)
	if !bytes.Equal(before, after) {
		t.Fatal("missing-key operations invoked native encryption")
	}
}
