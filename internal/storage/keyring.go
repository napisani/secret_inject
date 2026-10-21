package storage

import (
	"errors"
	"os"
	"path"
	"strings"

	keyring "github.com/99designs/keyring"
	"github.com/napisani/secret_inject/internal/config"
	"github.com/napisani/secret_inject/internal/secret"
)

type Keyring struct {
	keyring keyring.Keyring
}

func NewKeyring(storageConfig *config.StorageConfig) (*Keyring, error) {
	name := "secret_inject"

	allowedBackends := []keyring.BackendType{}
	availableBackends := keyring.AvailableBackends()
	for _, backend := range storageConfig.AllowedBackends {
		for _, availableBackend := range availableBackends {
			if backend == string(availableBackend) {
				allowedBackends = append(allowedBackends, availableBackend)
			}
		}
	}

	tmpDir := os.TempDir()
	filePath := path.Join(tmpDir, ".keyring.jwt")

	masterPassword := storageConfig.Password

	getPassword := func(prompt string) (string, error) {
		if masterPassword != "" {
			return masterPassword, nil
		}
		return getPasswordStdin(prompt)
	}

	keyringConfig := keyring.Config{
		AllowedBackends:                allowedBackends,
		ServiceName:                    name,
		KeychainTrustApplication:       true,
		KeychainName:                   name,
		KWalletAppID:                   name,
		KeychainSynchronizable:         false,
		KeychainAccessibleWhenUnlocked: true,
		FilePasswordFunc:               getPassword,
		KeychainPasswordFunc:           getPassword,
		FileDir:                        filePath,
	}

	kr, err := keyring.Open(keyringConfig)

	if err != nil {
		return nil, err
	}

	return &Keyring{
		keyring: kr,
	}, nil
}

func (s *Keyring) Initialize() error {
	marker, err := s.keyring.Get(migrationMarker)
	if err == nil {
		if string(marker.Data) != markerContent {
			return errors.New("invalid cache migration marker")
		}
		return nil
	}
	if !errors.Is(err, keyring.ErrKeyNotFound) {
		return err
	}
	if err := s.keyring.Remove("secret_inject"); err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
		return err
	}
	return s.keyring.Set(keyring.Item{Key: migrationMarker, Data: []byte(markerContent)})
}

func (s *Keyring) Read(scope string) (*secret.Secrets, bool, error) {
	key, err := cacheKey(scope)
	if err != nil {
		return nil, false, err
	}
	item, err := s.keyring.Get(key)
	if errors.Is(err, keyring.ErrKeyNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	snapshot, err := decodeSnapshot(item.Data)
	return snapshot, err == nil, err
}

func (s *Keyring) Write(scope string, snapshot *secret.Secrets) error {
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
	return s.keyring.Set(keyring.Item{Key: key, Data: data})
}

func (s *Keyring) CleanAll() error {
	keys, err := s.keyring.Keys()
	if err != nil {
		return err
	}
	for _, key := range keys {
		if !strings.HasPrefix(key, cachePrefix) || !scopePattern.MatchString(strings.TrimPrefix(key, cachePrefix)) {
			continue
		}
		if err := s.keyring.Remove(key); err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
			return err
		}
	}
	return nil
}
