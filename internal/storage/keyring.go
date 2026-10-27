package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/napisani/secret_inject/internal/privatefs"
)

// CleanLegacyEncryptedFile removes only app-owned cache entries without decoding.
func CleanLegacyEncryptedFile(directory string) (err error) {
	if _, err := os.Lstat(directory); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return ErrStoreOperation
	}
	if err := privatefs.EnsureDir(directory); err != nil {
		return ErrStoreUnavailable
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return ErrStoreUnavailable
	}
	defer func() { err = errors.Join(err, safeStoreFailure(root.Close())) }()
	entries, err := os.ReadDir(directory)
	if err != nil {
		return ErrStoreOperation
	}
	for _, entry := range entries {
		name := entry.Name()
		owned := name == "secret_inject" || name == cacheVersionKey || strings.HasPrefix(name, cachePrefix) && scopePattern.MatchString(strings.TrimPrefix(name, cachePrefix))
		if !owned {
			continue
		}
		info, err := root.Lstat(name)
		if err != nil {
			return ErrStoreOperation
		}
		if !info.Mode().IsRegular() {
			return ErrStoreOperation
		}
		file, err := privatefs.Open(filepath.Join(directory, name), os.O_RDONLY)
		if err != nil {
			return ErrStoreUnavailable
		}
		if err := file.Close(); err != nil {
			return ErrStoreOperation
		}
		if err := root.Remove(name); err != nil {
			return ErrStoreOperation
		}
	}
	return nil
}
