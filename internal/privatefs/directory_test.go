package privatefs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/napisani/secret_inject/internal/privatefs"
)

func TestOpenDirNeverCreatesAndRejectsSymlinks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	missing := filepath.Join(root, "missing")
	if file, err := privatefs.OpenDir(missing); err == nil {
		file.Close()
		t.Fatal("missing directory was opened")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("directory validation created a directory")
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if file, err := privatefs.OpenDir(link); err == nil {
		file.Close()
		t.Fatal("directory validation followed a symlink")
	}
}
