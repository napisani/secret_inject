package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	keyring "github.com/99designs/keyring"
	"github.com/napisani/secret_inject/internal/secret"
)

func TestStorageScopedLifecycleAndVersion(t *testing.T) {
	for _, kind := range []string{"file", "keyring"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			legacy := filepath.Join(root, "legacy")
			ring := &keyring.ArrayKeyring{}
			var store Storage
			if kind == "file" {
				if err := os.WriteFile(legacy, []byte("not valid JSON, must never be read"), 0600); err != nil {
					t.Fatal(err)
				}
				store = &File{directory: filepath.Join(root, "cache"), legacyPath: legacy}
			} else {
				if err := ring.Set(keyring.Item{Key: "secret_inject", Data: []byte("not JSON")}); err != nil {
					t.Fatal(err)
				}
				store = &Keyring{keyring: ring}
			}
			if err := store.Initialize(); err != nil {
				t.Fatal(err)
			}
			if err := store.Initialize(); err != nil {
				t.Fatal(err)
			}
			if kind == "file" {
				if _, err := os.Stat(legacy); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("legacy file remains")
				}
			} else if _, err := ring.Get("secret_inject"); !errors.Is(err, keyring.ErrKeyNotFound) {
				t.Fatal("legacy key remains")
			}
			a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
			if _, found, err := store.Read(a); err != nil || found {
				t.Fatal("expected miss")
			}
			s := secret.New()
			s.Entries["KEY"] = "a longer synthetic value"
			if err := store.Write(a, s); err != nil {
				t.Fatal(err)
			}
			s.Entries["KEY"] = "x"
			if err := store.Write(a, s); err != nil {
				t.Fatal(err)
			}
			if err := store.Write(b, s); err != nil {
				t.Fatal(err)
			}
			got, found, err := store.Read(a)
			if err != nil || !found || got.Entries["KEY"] != "x" {
				t.Fatal("snapshot replacement failed")
			}
			if kind == "file" {
				if err := os.WriteFile(filepath.Join(root, "cache", "unrelated"), []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			} else if err := ring.Set(keyring.Item{Key: "unrelated", Data: []byte("keep")}); err != nil {
				t.Fatal(err)
			}
			if err := store.CleanAll(); err != nil {
				t.Fatal(err)
			}
			for _, scope := range []string{a, b} {
				if _, found, err := store.Read(scope); err != nil || found {
					t.Fatal("scoped entry not cleaned")
				}
			}
			if err := store.Initialize(); err != nil {
				t.Fatal(err)
			}
			if kind == "file" {
				if _, err := os.Stat(filepath.Join(root, "cache", "unrelated")); err != nil {
					t.Fatal(err)
				}
			} else if _, err := ring.Get("unrelated"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFileKeyringCacheVersion(t *testing.T) {
	directory := t.TempDir()
	raw, err := keyring.Open(keyring.Config{
		AllowedBackends: []keyring.BackendType{keyring.FileBackend},
		FileDir:         directory,
		FilePasswordFunc: func(string) (string, error) {
			return "synthetic fixture password", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &Keyring{keyring: raw}
	if err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	scope := strings.Repeat("a", 64)
	for key, data := range map[string]string{
		cachePrefix + scope: "opaque old snapshot",
		cacheVersionKey:     "old",
	} {
		if err := raw.Set(keyring.Item{Key: key, Data: []byte(data)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.Read(scope); err != nil || found {
		t.Fatal("old-version cache was not discarded")
	}
	if err := os.WriteFile(filepath.Join(directory, cacheVersionKey), []byte("not an encrypted item"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.CleanAll(); err != nil {
		t.Fatal("cleanup tried to decode old cache metadata:", err)
	}
	if err := store.Initialize(); err != nil {
		t.Fatal("cleanup did not write the current version:", err)
	}
}

func TestFileResetFailureAndCleanupBoundaries(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "legacy")
	if err := os.Mkdir(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	store := &File{directory: filepath.Join(root, "cache"), legacyPath: legacy}
	if err := store.Initialize(); err == nil {
		t.Fatal("cache reset deleted a directory")
	}
	if _, err := os.Stat(filepath.Join(store.directory, cacheVersionKey)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed reset recorded the current version")
	}
	if err := os.Remove(legacy); err != nil {
		t.Fatal(err)
	}
	if err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	name := cachePrefix + strings.Repeat("d", 64) + ".json"
	if err := os.Mkdir(filepath.Join(store.directory, name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := store.CleanAll(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.directory, name)); err != nil {
		t.Fatal("cleanup deleted a directory")
	}
}

type failingKeyring struct {
	keyring.Keyring
	operation string
}

func (r *failingKeyring) Remove(key string) error {
	if r.operation == "remove" {
		return errors.New("backend failure")
	}
	return r.Keyring.Remove(key)
}
func (r *failingKeyring) Set(item keyring.Item) error {
	if r.operation == "set" {
		return errors.New("backend failure")
	}
	return r.Keyring.Set(item)
}
func (r *failingKeyring) Keys() ([]string, error) {
	if r.operation == "keys" {
		return nil, errors.New("backend failure")
	}
	return r.Keyring.Keys()
}

func TestKeyringResetErrorsPropagate(t *testing.T) {
	for _, operation := range []string{"remove", "set", "keys"} {
		raw := &keyring.ArrayKeyring{}
		if err := raw.Set(keyring.Item{Key: "secret_inject", Data: []byte("opaque")}); err != nil {
			t.Fatal(err)
		}
		ring := &failingKeyring{Keyring: raw, operation: operation}
		store := &Keyring{keyring: ring}
		if err := store.Initialize(); err == nil {
			t.Fatal("cache reset error suppressed")
		}
		if _, err := raw.Get(cacheVersionKey); !errors.Is(err, keyring.ErrKeyNotFound) {
			t.Fatal("failed reset recorded the current version")
		}
		ring.operation = ""
		if err := store.Initialize(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCorruptSnapshotsAndOldCacheVersion(t *testing.T) {
	ring := &keyring.ArrayKeyring{}
	store := &Keyring{keyring: ring}
	if err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	scope := strings.Repeat("c", 64)
	for _, data := range []string{"", `{}`, `{"entries":{},"timestamp":"0001-01-01T00:00:00Z"}`, "not JSON"} {
		if err := ring.Set(keyring.Item{Key: "sources-v1-" + scope, Data: []byte(data)}); err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.Read(scope); err == nil {
			t.Fatal("corrupt cache treated as hit/miss")
		}
	}
	if err := ring.Set(keyring.Item{Key: cacheVersionKey, Data: []byte("old")}); err != nil {
		t.Fatal(err)
	}
	if err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.Read(scope); err != nil || found {
		t.Fatal("old-version snapshot was not discarded")
	}
	if err := store.Write("../escape", secret.New()); err == nil {
		t.Fatal("unsafe scope accepted")
	}
}
