package storage

import (
	"fmt"
	"log/slog"
	"os"
	"path"

	"github.com/napisani/secret_inject/internal/secret"
)

const fileName = ".secret_inject.cache"

var tmpDir = os.TempDir()
var fullFilePath = path.Join(tmpDir, fileName)

type File struct{}

func NewFile() *File {
	// Security warning only in debug mode to avoid polluting output
	slog.Debug("Using file storage - secrets stored in PLAINTEXT",
		"location", fullFilePath,
		"warning", "Not secure for production use - consider using keyring storage")
	return &File{}
}

func (s *File) HasCachedSecrets() bool {
	_, err := os.Stat(fullFilePath)
	return !os.IsNotExist(err)
}

func (s *File) GetCachedSecrets() (*secret.Secrets, error) {
	slog.Debug("Reading cached secrets", "path", fullFilePath)

	fileInfo, err := os.Stat(fullFilePath)
	if os.IsNotExist(err) || fileInfo.Size() == 0 {
		return nil, err
	}

	// Check file permissions
	mode := fileInfo.Mode()
	if mode.Perm() != 0600 {
		slog.Debug("Cache file has insecure permissions",
			"path", fullFilePath,
			"current", fmt.Sprintf("%04o", mode.Perm()),
			"recommended", "0600")
	}

	content, err := os.ReadFile(fullFilePath)
	if err != nil {
		return nil, err
	}
	slog.Debug("Read cached secrets", "bytes", len(content))

	return secret.Deserialize(content)
}

func (s *File) CacheSecrets(secrets *secret.Secrets) error {
	err := mkdirRecursive(tmpDir)
	if err != nil {
		return err
	}

	file, err := createFileIfNotExist(fullFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Ensure file has secure permissions (owner read/write only)
	err = file.Chmod(0600)
	if err != nil {
		slog.Debug("Failed to set secure file permissions", "error", err)
	}

	slog.Debug("Caching secrets", "path", fullFilePath, "count", len(secrets.Entries))
	serializedSecrets, err := secrets.Serialize()
	if err != nil {
		return err
	}

	_, err = file.Write(serializedSecrets)
	if err != nil {
		return err
	}

	return nil
}

func (s *File) CleanCachedSecrets() error {
	err := os.Remove(fullFilePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	slog.Debug("Removed cached secrets", "path", fullFilePath)
	return nil
}

// Helper functions
func mkdirRecursive(path string) error {
	slog.Debug("Creating directory (if does not exist)", "path", path)
	err := os.MkdirAll(path, 0755)
	if err != nil {
		return err
	}
	slog.Debug("Created directory", "path", path)
	return nil
}

func createFileIfNotExist(filename string) (*os.File, error) {
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		slog.Debug("Creating file", "filename", filename)
		file, err := os.Create(filename)
		if err != nil {
			return file, err
		}
		return file, nil
	}

	slog.Debug("Opening file", "filename", filename)
	return os.OpenFile(filename, os.O_RDWR|os.O_TRUNC, 0600)
}
