package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/napisani/secret_inject/internal/secret"
)

func TestLegacyFileStorageScopedLifecycle(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "legacy")
	if err := os.WriteFile(legacy, []byte("opaque obsolete cache"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &File{directory: filepath.Join(root, "cache"), legacyPath: legacy}
	if err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("old cache remains")
	}
	scope := strings.Repeat("a", 64)
	snapshot := secret.New()
	snapshot.Entries["KEY"] = "VALUE"
	if err := store.Write(scope, snapshot); err != nil {
		t.Fatal(err)
	}
	if got, found, err := store.Read(scope); err != nil || !found || got.Entries["KEY"] != "VALUE" {
		t.Fatal("legacy fixture roundtrip failed")
	}
	if err := os.WriteFile(filepath.Join(store.directory, "unrelated"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.CleanAll(); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.Read(scope); err != nil || found {
		t.Fatal("cleanup retained a cache entry")
	}
	if _, err := os.Stat(filepath.Join(store.directory, "unrelated")); err != nil {
		t.Fatal("cleanup removed unrelated data")
	}
}

func TestLegacyEncryptedCleanupDoesNotDecodeOrTouchOtherRoles(t *testing.T) {
	directory := t.TempDir()
	owned := []string{"secret_inject", cacheVersionKey, cachePrefix + strings.Repeat("a", 64)}
	for _, name := range append(append([]string{}, owned...), "unrelated", "bootstrap-v1-root") {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("not an encrypted payload"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := CleanLegacyEncryptedFile(directory); err != nil {
		t.Fatal(err)
	}
	for _, name := range owned {
		if _, err := os.Stat(filepath.Join(directory, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("owned cache remains")
		}
	}
	for _, name := range []string{"unrelated", "bootstrap-v1-root"} {
		if _, err := os.Stat(filepath.Join(directory, name)); err != nil {
			t.Fatal("cleanup crossed ownership")
		}
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

func TestCorruptLegacyFileSnapshotsAreNotHits(t *testing.T) {
	root := t.TempDir()
	store := &File{directory: filepath.Join(root, "cache"), legacyPath: filepath.Join(root, "legacy")}
	if err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	scope := strings.Repeat("c", 64)
	for _, data := range []string{"", `{}`, `{"entries":{},"timestamp":"0001-01-01T00:00:00Z"}`, "not JSON"} {
		if err := os.WriteFile(filepath.Join(store.directory, cachePrefix+scope+".json"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.Read(scope); err == nil {
			t.Fatal("corrupt snapshot accepted")
		}
	}
	if err := store.Write("../escape", secret.New()); err == nil {
		t.Fatal("unsafe scope accepted")
	}
}
