package main

import (
	"log/slog"

	keyring "github.com/99designs/keyring"
)

type KeyringStorage struct {
	keyring keyring.Keyring
}

func (s *KeyringStorage) HasCachedSecrets() bool {
	keys, err := s.keyring.Keys()
	if err != nil {
		slog.Debug("Error getting keys: %s", err)
		return false
	}
	return len(keys) > 0
}

func (s *KeyringStorage) GetCachedSecrets() (*Secrets, error) {
	slog.Debug("Reading cached secrets from %s", fullFilePath)
	keys, err := s.keyring.Keys()
	if err != nil {
		return nil, err
	}
	secrets := NewSecrets()
	for _, key := range keys {
		value, err := s.keyring.Get(key)
		if err != nil {
			return nil, err
		}
		secrets.Entries[key] = string(value.Data)
	}

	slog.Debug("Read cached secrets: %s", secrets)
	return secrets, nil
}

func (s *KeyringStorage) CacheSecrets(secrets *Secrets) error {
	slog.Debug("Secrets: %s", secrets)
	for key, value := range secrets.Entries {
		err := s.keyring.Set(keyring.Item{
			Key:  key,
			Data: []byte(value),
		})
		if err != nil {
			slog.Debug("Error caching secrets: %s", err)
			return err
		}
	}
	slog.Debug("Cached secrets to keyring")
	return nil
}

func (s *KeyringStorage) CleanCachedSecrets() error {
	keys, err := s.keyring.Keys()
	if err != nil {
		return err
	}
	for _, key := range keys {
		err := s.keyring.Remove(key)
		if err != nil {
			return err
		}
	}
	slog.Debug("Removed cached secrets from keyring: %s", keys)
	return nil
}

func NewKeyringStorage() (*KeyringStorage, error) {
	keyring, err := keyring.Open(keyring.Config{
		ServiceName:              "secret_inject",
		KeychainTrustApplication: true,
		KeychainName:             "secret_inject",
    KeychainSynchronizable: false,
    KeychainAccessibleWhenUnlocked: true,
	})

	if err != nil {
		return nil, err
	}

	return &KeyringStorage{
		keyring: keyring,
	}, nil
}
