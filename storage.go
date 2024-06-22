package main

import (
	"errors"
	"log/slog"
)

type Storage interface {
	HasCachedSecrets() bool
	GetCachedSecrets() (*Secrets, error)
	CacheSecrets(secrets *Secrets) error
	CleanCachedSecrets() error
}

func GetStorage(fullConfig map[string]interface{}) (Storage, error) {
	storageConfig, ok := fullConfig["storage"].(map[string]interface{})
	if !ok {
		return nil, errors.New("No storage configuration found")
	}

	storageType := storageConfig["type"].(string)
	switch storageType {
	case "keyring":
		slog.Debug("Using keyring storage")
		return NewKeyringStorage()
	case "file":
		slog.Debug("Using file storage")
		return NewInsecureFileStorage(), nil
	default:
		return nil, errors.New("Unknown storage type")
	}
}
