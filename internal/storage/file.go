package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/napisani/secret_inject/internal/privatefs"
	"github.com/napisani/secret_inject/internal/secret"
)

type File struct {
	directory  string
	legacyPath string
}

func NewFile() (*File, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	return &File{
		directory:  filepath.Join(root, "secret_inject"),
		legacyPath: filepath.Join(os.TempDir(), ".secret_inject.cache"),
	}, nil
}

func (s *File) Initialize() error {
	if err := privatefs.EnsureDir(s.directory); err != nil {
		return err
	}
	version, err := privatefs.ReadFile(filepath.Join(s.directory, cacheVersionKey))
	if err == nil && string(version) == cacheVersion {
		return nil
	}
	return s.CleanAll()
}

func (s *File) Read(scope string) (*secret.Secrets, bool, error) {
	key, err := cacheKey(scope)
	if err != nil {
		return nil, false, err
	}
	data, err := privatefs.ReadFile(filepath.Join(s.directory, key+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	snapshot, err := decodeSnapshot(data)
	return snapshot, err == nil, err
}

func (s *File) Write(scope string, snapshot *secret.Secrets) error {
	key, err := cacheKey(scope)
	if err != nil {
		return err
	}
	data, err := snapshot.Serialize()
	if err != nil {
		return err
	}
	if _, err := decodeSnapshot(data); err != nil {
		return err
	}
	return privatefs.WriteAtomic(s.directory, key+".json", data)
}

func (s *File) CleanAll() error {
	if err := privatefs.EnsureDir(s.directory); err != nil {
		return err
	}
	if info, err := os.Lstat(s.legacyPath); err == nil && info.IsDir() {
		return errors.New("legacy cache path is a directory")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(s.legacyPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	entries, err := os.ReadDir(s.directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		scope := strings.TrimSuffix(strings.TrimPrefix(name, cachePrefix), ".json")
		if name != cachePrefix+scope+".json" || !scopePattern.MatchString(scope) {
			continue
		}
		if err := os.Remove(filepath.Join(s.directory, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return privatefs.WriteAtomic(s.directory, cacheVersionKey, []byte(cacheVersion))
}
