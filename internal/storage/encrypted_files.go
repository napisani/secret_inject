package storage

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/napisani/secret_inject/internal/privatefs"
)

type encryptedFiles struct {
	directory, extension    string
	valueLimit, cipherLimit int
	encrypt, decrypt        func(context.Context, string, []byte) ([]byte, error)
}

func (s *encryptedFiles) MaxValueBytes() int { return s.valueLimit }
func (*encryptedFiles) Close() error         { return nil }

func encryptedObjectName(key, extension string) (string, error) {
	if key == "" || len(key) > 180 {
		return "", ErrStoreOperation
	}
	return base64.RawURLEncoding.EncodeToString([]byte(key)) + extension, nil
}

func (s *encryptedFiles) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	name, err := encryptedObjectName(key, s.extension)
	if err != nil {
		return nil, false, err
	}
	file, err := privatefs.Open(filepath.Join(s.directory, name), os.O_RDONLY)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, ErrStoreOperation
	}
	cipher, readErr := io.ReadAll(io.LimitReader(file, int64(s.cipherLimit)+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(cipher) > s.cipherLimit {
		return nil, false, ErrStoreOperation
	}
	plain, err := s.decrypt(ctx, name, cipher)
	if err != nil {
		return nil, false, err
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if len(plain) > s.valueLimit {
		return nil, false, ErrRecordCorrupt
	}
	return plain, true, nil
}

func (s *encryptedFiles) Put(ctx context.Context, key string, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(value) > s.valueLimit {
		return ErrStoreOperation
	}
	name, err := encryptedObjectName(key, s.extension)
	if err != nil {
		return err
	}
	file, err := privatefs.Open(filepath.Join(s.directory, name), os.O_RDONLY)
	if err != nil && !os.IsNotExist(err) {
		return ErrStoreOperation
	}
	if err == nil {
		if err := file.Close(); err != nil {
			return ErrStoreOperation
		}
	}
	cipher, err := s.encrypt(ctx, name, value)
	if err != nil {
		return err
	}
	if len(cipher) > s.cipherLimit {
		return ErrStoreOperation
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := privatefs.WriteAtomic(s.directory, name, cipher); err != nil {
		return safeStoreFailure(err)
	}
	return errors.Join(syncPrivateDirectory(s.directory), ctx.Err())
}

func (s *encryptedFiles) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name, err := encryptedObjectName(key, s.extension)
	if err != nil {
		return err
	}
	path := filepath.Join(s.directory, name)
	file, err := privatefs.Open(path, os.O_RDONLY)
	if os.IsNotExist(err) {
		return syncPrivateDirectory(s.directory)
	}
	if err != nil {
		return ErrStoreOperation
	}
	if err := file.Close(); err != nil {
		return ErrStoreOperation
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return ErrStoreOperation
	}
	return errors.Join(syncPrivateDirectory(s.directory), ctx.Err())
}

func syncPrivateDirectory(path string) error {
	directory, err := privatefs.OpenDir(path)
	if err != nil {
		return ErrStoreOperation
	}
	return safeStoreFailure(errors.Join(directory.Sync(), directory.Close()))
}

func (s *encryptedFiles) Keys(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	directory, err := privatefs.OpenDir(s.directory)
	if err != nil {
		return nil, ErrStoreUnavailable
	}
	entries, readErr := directory.ReadDir(-1)
	closeErr := directory.Close()
	if readErr != nil || closeErr != nil {
		return nil, ErrStoreOperation
	}
	keys := []string{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), s.extension) {
			continue
		}
		if !entry.Type().IsRegular() {
			return nil, ErrStoreOperation
		}
		key, err := base64.RawURLEncoding.DecodeString(strings.TrimSuffix(entry.Name(), s.extension))
		if err != nil {
			return nil, ErrRecordCorrupt
		}
		name, err := encryptedObjectName(string(key), s.extension)
		if err != nil || name != entry.Name() {
			return nil, ErrRecordCorrupt
		}
		keys = append(keys, string(key))
	}
	return keys, nil
}
