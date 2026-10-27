package privatefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateFilesAndAtomicReplacement(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := EnsureDir(root); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(root, "data", []byte("long synthetic value")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(root, "data", []byte("x")); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFile(filepath.Join(root, "data"))
	if err != nil || string(got) != "x" {
		t.Fatal("private replacement failed")
	}
	for path, mode := range map[string]os.FileMode{root: 0700, filepath.Join(root, "data"): 0600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatal("private permissions not enforced")
		}
	}
	target := filepath.Join(root, "data")
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Skip("symlink creation unavailable")
	}
	if _, err := ReadFile(alias); err == nil {
		t.Fatal("symlink followed")
	}
	directoryAlias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, directoryAlias); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDir(directoryAlias); err == nil {
		t.Fatal("symlink directory accepted")
	}
}
