package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"github.com/napisani/secret_inject/internal/privatefs"
)

const (
	ageValueLimit    = 64 * 1024
	ageCipherLimit   = 2 * ageValueLimit
	ageIdentityLimit = 1024
	ageRecordPrefix  = "secret-inject-age-record-v1\n"
)

type ageFiles struct {
	*encryptedFiles
	identityPath, recipient string
}

func openAgeFiles(ctx context.Context, directory string, policy StorePolicy) (NativeByteAdapter, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	records := filepath.Join(directory, "records")
	for _, path := range []string{directory, records} {
		if policy == InteractiveSetup {
			if err := privatefs.EnsureDir(path); err != nil {
				return nil, ErrStoreUnavailable
			}
			continue
		}
		if policy != Unattended {
			return nil, ErrStoreUnavailable
		}
		file, err := privatefs.OpenDir(path)
		if os.IsNotExist(err) {
			return nil, ErrInteractionRequired
		}
		if err != nil {
			return nil, ErrStoreUnavailable
		}
		if err := file.Close(); err != nil {
			return nil, ErrStoreOperation
		}
	}
	identityPath := filepath.Join(directory, "identity")
	identity, err := readAgeIdentity(identityPath)
	if errors.Is(err, ErrInteractionRequired) && policy == InteractiveSetup {
		entries, readErr := os.ReadDir(records)
		if readErr != nil {
			return nil, ErrStoreUnavailable
		}
		// A replacement identity would strand existing ciphertext permanently.
		if len(entries) != 0 {
			return nil, ErrRecordCorrupt
		}
		if err := createAgeIdentity(ctx, directory); err != nil {
			return nil, err
		}
		identity, err = readAgeIdentity(identityPath)
	}
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s := &ageFiles{identityPath: identityPath, recipient: identity.Recipient().String()}
	s.encryptedFiles = &encryptedFiles{
		directory: records, extension: ".age",
		valueLimit: ageValueLimit, cipherLimit: ageCipherLimit,
		encrypt: s.encrypt, decrypt: s.decrypt,
	}
	return s, nil
}

func readAgeIdentity(path string) (*age.X25519Identity, error) {
	file, err := privatefs.Open(path, os.O_RDONLY)
	if os.IsNotExist(err) {
		return nil, ErrInteractionRequired
	}
	if err != nil {
		return nil, ErrStoreUnavailable
	}
	data, readErr := io.ReadAll(io.LimitReader(file, ageIdentityLimit+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(data) > ageIdentityLimit {
		return nil, ErrStoreUnavailable
	}
	identity, err := age.ParseX25519Identity(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, ErrStoreUnavailable
	}
	return identity, nil
}

func createAgeIdentity(ctx context.Context, directory string) (err error) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return ErrStoreOperation
	}
	temporary := filepath.Join(directory, ".identity-"+rand.Text())
	file, err := privatefs.Open(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
	if err != nil {
		return ErrStoreOperation
	}
	defer func() {
		cleanupErr := os.Remove(temporary)
		if cleanupErr != nil && !os.IsNotExist(cleanupErr) {
			err = errors.Join(err, ErrStoreOperation)
		}
	}()
	_, writeErr := io.WriteString(file, identity.String()+"\n")
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return ErrStoreOperation
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Linking publishes a complete, synced identity without replacing any key.
	if err := os.Link(temporary, filepath.Join(directory, "identity")); err != nil && !os.IsExist(err) {
		return ErrStoreOperation
	}
	return errors.Join(syncPrivateDirectory(directory), syncPrivateDirectory(filepath.Dir(directory)))
}

func (s *ageFiles) identity(ctx context.Context) (*age.X25519Identity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	identity, err := readAgeIdentity(s.identityPath)
	if err != nil {
		return nil, err
	}
	if identity.Recipient().String() != s.recipient {
		return nil, ErrStoreUnavailable
	}
	return identity, nil
}

func (s *ageFiles) encrypt(ctx context.Context, name string, value []byte) ([]byte, error) {
	identity, err := s.identity(ctx)
	if err != nil {
		return nil, err
	}
	var cipher bytes.Buffer
	writer, err := age.Encrypt(&cipher, identity.Recipient())
	if err != nil {
		return nil, ErrStoreOperation
	}
	_, prefixErr := io.WriteString(writer, ageRecordPrefix+name+"\n")
	_, writeErr := writer.Write(value)
	closeErr := writer.Close()
	if prefixErr != nil || writeErr != nil || closeErr != nil {
		return nil, ErrStoreOperation
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return cipher.Bytes(), nil
}

func (s *ageFiles) decrypt(ctx context.Context, name string, cipher []byte) ([]byte, error) {
	identity, err := s.identity(ctx)
	if err != nil {
		return nil, err
	}
	reader, err := age.Decrypt(bytes.NewReader(cipher), identity)
	if err != nil {
		return nil, ErrRecordCorrupt
	}
	prefix := []byte(ageRecordPrefix + name + "\n")
	limit := len(prefix) + ageValueLimit
	plain, err := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
	if err != nil || len(plain) > limit || !bytes.HasPrefix(plain, prefix) {
		return nil, ErrRecordCorrupt
	}
	return plain[len(prefix):], nil
}
