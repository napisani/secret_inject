package envfile

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/napisani/secret_inject/internal/secret"
)

func ValidatePatterns(patterns []string) error {
	for _, pattern := range patterns {
		if pattern == "" || pattern == "." || pattern == ".." || strings.ContainsAny(pattern, "/\\\x00") {
			return errors.New("env file patterns must name files in the current directory")
		}
		if _, err := filepath.Match(pattern, ""); err != nil {
			return errors.New("invalid env file pattern")
		}
	}
	return nil
}

func Load(patterns []string) (snapshot *secret.Secrets, err error) {
	if err := ValidatePatterns(patterns); err != nil {
		return nil, err
	}
	snapshot = secret.New()
	if len(patterns) == 0 {
		return snapshot, nil
	}
	root, err := os.OpenRoot(".")
	if err != nil {
		return nil, errors.New("local env directory unavailable")
	}
	defer func() {
		if closeErr := root.Close(); closeErr != nil && err == nil {
			snapshot, err = nil, errors.New("local env directory close failed")
		}
	}()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, errors.New("local env directory could not be read")
	}
	seen := make(map[string]bool)
	for _, pattern := range patterns {
		for _, entry := range entries {
			matched, err := filepath.Match(pattern, entry.Name())
			if err != nil {
				return nil, errors.New("invalid env file pattern")
			}
			if !matched || entry.IsDir() || seen[entry.Name()] {
				continue
			}
			if !entry.Type().IsRegular() {
				return nil, errors.New("local env inputs must be regular files, not symlinks")
			}
			file, err := root.Open(entry.Name())
			if err != nil {
				return nil, errors.New("local env file could not be opened")
			}
			values, parseErr := godotenv.Parse(file)
			closeErr := file.Close()
			if parseErr != nil {
				return nil, errors.New("local env file could not be parsed")
			}
			if closeErr != nil {
				return nil, errors.New("local env file close failed")
			}
			maps.Copy(snapshot.Entries, values)
			seen[entry.Name()] = true
		}
	}
	return snapshot, nil
}
