package storage

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptedFilesRejectUnsafeEntries(t *testing.T) {
	factories := []struct {
		name string
		open func(*testing.T) *encryptedFiles
	}{
		{name: "age", open: func(t *testing.T) *encryptedFiles { return ageFixture(t).encryptedFiles }},
		{name: "systemd", open: func(t *testing.T) *encryptedFiles {
			native, _ := systemdFixture(t, "normal")
			if err := native.prepare(t.Context(), InteractiveSetup); err != nil {
				t.Fatal(err)
			}
			return native.files()
		}},
	}
	for _, factory := range factories {
		t.Run(factory.name, func(t *testing.T) {
			s := factory.open(t)
			unrelated := filepath.Join(t.TempDir(), "unrelated")
			if err := os.WriteFile(unrelated, []byte("MUST_REMAIN_UNTOUCHED"), 0600); err != nil {
				t.Fatal(err)
			}
			for _, kind := range []string{"symlink", "directory"} {
				t.Run(kind, func(t *testing.T) {
					name, _ := encryptedObjectName(kind, s.extension)
					path := filepath.Join(s.directory, name)
					if kind == "symlink" {
						if err := os.Symlink(unrelated, path); err != nil {
							t.Fatal(err)
						}
					} else if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
					if _, _, err := s.Get(t.Context(), kind); !errors.Is(err, ErrStoreOperation) {
						t.Fatal("unsafe record was read")
					}
					if err := s.Put(t.Context(), kind, []byte("SYNTHETIC")); !errors.Is(err, ErrStoreOperation) {
						t.Fatal("unsafe record was replaced")
					}
					if err := s.Delete(t.Context(), kind); !errors.Is(err, ErrStoreOperation) {
						t.Fatal("unsafe record was deleted")
					}
					if _, err := s.Keys(t.Context()); !errors.Is(err, ErrStoreOperation) {
						t.Fatal("unsafe record was enumerated")
					}
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				})
			}
			value, err := os.ReadFile(unrelated)
			if err != nil || string(value) != "MUST_REMAIN_UNTOUCHED" {
				t.Fatal("unrelated file changed")
			}
			if err := os.WriteFile(filepath.Join(s.directory, "!"+s.extension), []byte("SYNTHETIC"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Keys(t.Context()); !errors.Is(err, ErrRecordCorrupt) {
				t.Fatal("malformed record filename was accepted")
			}
		})
	}
}

func TestEncryptedFilesBoundariesAndCancellation(t *testing.T) {
	t.Parallel()
	s := ageFixture(t).encryptedFiles
	for _, value := range [][]byte{{}, bytes.Repeat([]byte{0, 255, '\n', '\r'}, ageValueLimit/4)} {
		if err := s.Put(t.Context(), "../logical/name", value); err != nil {
			t.Fatal(err)
		}
		actual, found, err := s.Get(t.Context(), "../logical/name")
		if err != nil || !found || !bytes.Equal(value, actual) {
			t.Fatal("boundary value failed to roundtrip")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := s.Put(ctx, "must-not-exist", []byte("SYNTHETIC")); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled write succeeded")
	}
	if _, found, err := s.Get(t.Context(), "must-not-exist"); err != nil || found {
		t.Fatal("canceled write persisted data")
	}
}
