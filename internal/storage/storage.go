package storage

import (
	"errors"
	"log/slog"

	"github.com/napisani/secret_inject/internal/config"
	"github.com/napisani/secret_inject/internal/secret"
)

type Storage interface {
	HasCachedSecrets() bool
	GetCachedSecrets() (*secret.Secrets, error)
	CacheSecrets(secrets *secret.Secrets) error
	CleanCachedSecrets() error
}

func Get(storageConfig *config.StorageConfig) (Storage, error) {
	if storageConfig == nil {
		return nil, errors.New("no storage configuration found in config file")
	}

	switch storageConfig.Type {
	case "keyring":
		slog.Debug("Using keyring storage")
		return NewKeyring(storageConfig)
	case "file":
		slog.Debug("Using file storage")
		return NewFile(), nil
	default:
		return nil, errors.New("unknown storage type: must be 'keyring' or 'file'")
	}
}
