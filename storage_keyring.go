package main

import (
	"log/slog"

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
	slog.Debug("Reading cached secrets from %s", fullFilePath)
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

func NewKeyringStorage() (*KeyringStorage, error) {
	keyring, err := keyring.Open(keyring.Config{
		ServiceName:                    "secret_inject",
		KeychainTrustApplication:       true,
		KeychainName:                   "secret_inject",
		KeychainSynchronizable:         false,
		KeychainAccessibleWhenUnlocked: true,
	})

	if err != nil {
		return nil, err
	}

	return &KeyringStorage{
		keyring: keyring,
	}, nil
}
