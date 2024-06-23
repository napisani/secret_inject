package main

import (
	"fmt"
	"log/slog"
	"os"
	"path"

	keyring "github.com/99designs/keyring"
)

const key = "secret_inject"

type KeyringStorage struct {
	keyring keyring.Keyring
}

func (s *KeyringStorage) HasCachedSecrets() bool {
	value, err := s.keyring.Get(key)
	if err != nil {
		slog.Debug("Error getting keys: %s", err)
		return false
	}
	return value.Data != nil
}

func (s *KeyringStorage) GetCachedSecrets() (*Secrets, error) {
	slog.Debug("Reading cached secrets from keyring")
	value, err := s.keyring.Get(key)
	if err != nil {
		slog.Debug("Error getting key: %s", err)
		return nil, err
	}

	secrets, err := Deserialize(value.Data)
	if err != nil {
		return nil, err
	}
	slog.Debug("Read cached secrets: %s", secrets)
	return secrets, nil
}

func (s *KeyringStorage) CacheSecrets(secrets *Secrets) error {
	slog.Debug("Secrets: %s", secrets)
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

func (s *KeyringStorage) CleanCachedSecrets() error {
	if !s.HasCachedSecrets() {
		slog.Debug("No cached secrets to clean")
		return nil
	}

	slog.Debug("Removing cached secrets from keyring: %s", key)
	return s.keyring.Remove(key)
}

func NewKeyringStorage(storageConfig map[string]interface{}) (*KeyringStorage, error) {
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

	slog.Debug("Allowed backends: %s", allowedBackends)

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
		return GetPasswordStdin(prompt)
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

	slog.Debug("Keyring config: %s", keyringConfig)
	keyring, err := keyring.Open(keyringConfig)

	if err != nil {
		return nil, err
	}

	return &KeyringStorage{
		keyring: keyring,
	}, nil
}
