package storage

import (
	"fmt"
	"log/slog"
	"os"
	"path"

	keyring "github.com/99designs/keyring"
	"github.com/napisani/secret_inject/internal/secret"
)

const key = "secret_inject"

type Keyring struct {
	keyring keyring.Keyring
}

func NewKeyring(storageConfig map[string]interface{}) (*Keyring, error) {
	name := "secret_inject"

	allowedBackends := []keyring.BackendType{}
	allowedBackendsRaw, ok := storageConfig["allowed_backends"].([]interface{})
	if !ok || len(allowedBackendsRaw) == 0 {
		allowedBackendsRaw = nil
	} else {
		availableBackends := keyring.AvailableBackends()
		for _, backend := range allowedBackendsRaw {
			str := fmt.Sprintf("%v", backend)
			for _, availableBackend := range availableBackends {
				if str == string(availableBackend) {
					allowedBackends = append(allowedBackends, availableBackend)
				}
			}
		}
	}

	slog.Debug("Allowed backends", "backends", allowedBackends)

	tmpDir := os.TempDir()
	filePath := path.Join(tmpDir, ".keyring.jwt")

	masterPassword, ok := storageConfig["password"]
	if !ok {
		masterPassword = ""
	}

	getPassword := func(prompt string) (string, error) {
		if masterPassword != "" {
			return fmt.Sprintf("%v", masterPassword), nil
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

	slog.Debug("Keyring config", "service", name)
	kr, err := keyring.Open(keyringConfig)

	if err != nil {
		return nil, err
	}

	return &Keyring{
		keyring: kr,
	}, nil
}

func (s *Keyring) HasCachedSecrets() bool {
	value, err := s.keyring.Get(key)
	if err != nil {
		slog.Debug("Error getting keys", "error", err)
		return false
	}
	return value.Data != nil
}

func (s *Keyring) GetCachedSecrets() (*secret.Secrets, error) {
	slog.Debug("Reading cached secrets from keyring")
	value, err := s.keyring.Get(key)
	if err != nil {
		slog.Debug("Error getting key", "error", err)
		return nil, err
	}

	secrets, err := secret.Deserialize(value.Data)
	if err != nil {
		return nil, err
	}
	slog.Debug("Read cached secrets", "count", len(secrets.Entries))
	return secrets, nil
}

func (s *Keyring) CacheSecrets(secrets *secret.Secrets) error {
	slog.Debug("Caching secrets", "count", len(secrets.Entries))
	serializedSecrets, err := secrets.Serialize()
	if err != nil {
		return err
	}

	err = s.keyring.Set(keyring.Item{Key: key, Data: serializedSecrets})
	if err != nil {
		return err
	}

	slog.Debug("Cached secrets to keyring")
	return nil
}

func (s *Keyring) CleanCachedSecrets() error {
	if !s.HasCachedSecrets() {
		slog.Debug("No cached secrets to clean")
		return nil
	}

	slog.Debug("Removing cached secrets from keyring", "key", key)
	return s.keyring.Remove(key)
}
