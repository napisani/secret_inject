package privatefs

import (
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// EnsureDir requires an absolute, nonsymlink directory owned by the current user.
func EnsureDir(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("private directory must be absolute")
	}
	if err := mkdirNative(path); err != nil {
		return err
	}
	f, err := openNative(path, os.O_RDONLY, true)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err == nil && !info.IsDir() {
		err = errors.New("private path is not a directory")
	}
	if err == nil {
		err = protect(f, 0700)
	}
	return errors.Join(err, f.Close())
}

// Open refuses symlinks and nonregular files before enforcing current-user access.
func Open(path string, flags int) (*os.File, error) {
	f, err := openNative(path, flags, false)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = errors.New("private file must be regular")
	}
	if err == nil {
		err = protect(f, 0600)
	}
	if err != nil {
		return nil, errors.Join(err, f.Close())
	}
	if flags&os.O_APPEND != 0 {
		if _, err := f.Seek(0, io.SeekEnd); err != nil {
			return nil, errors.Join(err, f.Close())
		}
	}
	return f, nil
}

func ReadFile(path string) ([]byte, error) {
	f, err := Open(path, os.O_RDONLY)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(f)
	closeErr := f.Close()
	return data, errors.Join(readErr, closeErr)
}

func WriteAtomic(directory, name string, data []byte) (err error) {
	temp := filepath.Join(directory, ".write-"+rand.Text())
	f, err := Open(temp, os.O_CREATE|os.O_EXCL|os.O_RDWR)
	if err != nil {
		return err
	}
	defer func() {
		if cleanupErr := os.Remove(temp); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
			err = errors.Join(err, cleanupErr)
		}
	}()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	return os.Rename(temp, filepath.Join(directory, name))
}
