package diagnostics

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/napisani/secret_inject/internal/privatefs"
)

func TestPrivateLogRotationAndLock(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", root)
	session, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if other, err := Open(); err == nil {
		other.Finish()
		t.Fatal("concurrent debug writer acquired lock")
	}
	session.Record(Event{Kind: Unmatched, Mode: "include", Rule: "MISSING"})
	if err := session.Finish(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "secret_inject", "debug.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil || record["rule"] != "MISSING" {
		t.Fatal("missing structured diagnostic")
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxSize)), 0600); err != nil {
		t.Fatal(err)
	}
	session, err = Open()
	if err != nil {
		t.Fatal(err)
	}
	session.Record(Event{Kind: RenameCollision, Destination: "API_KEY", Winner: "SERVICE_TOKEN", Discarded: "API_KEY"})
	if err := session.Finish(); err != nil {
		t.Fatal(err)
	}
	backup, err := os.Stat(path + ".1")
	if err != nil || backup.Size() != maxSize {
		t.Fatal("rotation backup missing")
	}
	active, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(active), "rename-collision") {
		t.Fatal("rotation lost active event")
	}
}

func TestExistingBackupIsPrivateAndSymlinkTargetsAreRefused(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", root)
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "secret_inject", "debug.log")
	if err := privatefs.WriteAtomic(filepath.Dir(path), "debug.log.1", []byte("prior metadata")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path+".1", 0644); err != nil {
		t.Fatal(err)
	}
	s, err = Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path + ".1")
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("existing backup permissions not enforced")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path+".1", path); err != nil {
		t.Skip("symlink creation unavailable")
	}
	if s, err := Open(); err == nil {
		s.Finish()
		t.Fatal("symlink log target accepted")
	}
}

func TestDebugErrorsRetainCausesWithoutExposingPaths(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "private-path-sentinel")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", blocker)
	_, err := Open()
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatal("debug initialization discarded filesystem error cause")
	}
	if strings.Contains(err.Error(), blocker) {
		t.Fatal("debug initialization exposed private path")
	}

	t.Setenv("XDG_STATE_HOME", root)
	session, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := session.file.Close(); err != nil {
		t.Fatal(err)
	}
	session.Record(Event{Kind: Unmatched, Rule: "MISSING"})
	err = session.Finish()
	if !errors.Is(err, os.ErrClosed) {
		t.Fatal("debug writer discarded close failure cause")
	}
	if strings.Contains(err.Error(), root) {
		t.Fatal("debug finish exposed private path")
	}
}

func TestDebugFailuresAndPaths(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "relative")
	if s, err := Open(); err == nil {
		s.Finish()
		t.Fatal("relative state path accepted")
	}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.file.Close(); err != nil {
		t.Fatal(err)
	}
	s.Record(Event{Kind: Unmatched, Rule: "MISSING"})
	if err := s.Finish(); err == nil {
		t.Fatal("debug I/O failure swallowed")
	}
	s, err = Open()
	if err != nil {
		t.Fatal("failed session leaked lock")
	}
	s.Finish()
}
