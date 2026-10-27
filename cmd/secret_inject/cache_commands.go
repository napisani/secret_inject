package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/napisani/secret_inject/internal/config"
	"github.com/napisani/secret_inject/internal/storage"
)

func cleanConfiguredCache(ctx context.Context, cfg *config.Config) error {
	if storage.ValidateNativeConfig(cfg.Storage) == nil {
		store, err := openProtectedStore(ctx, cfg.Storage, storage.Unattended)
		if err != nil {
			return err
		}
		return errors.Join(store.CleanSnapshots(ctx), store.Close())
	}
	if cfg.Storage == nil {
		return storage.ErrStoreUnavailable
	}
	if cfg.Storage.Type == "file" {
		store, err := storage.NewFile()
		if err != nil {
			return err
		}
		return store.CleanAll()
	}
	if cfg.Storage.Type == "keyring" && len(cfg.Storage.AllowedBackends) == 1 && cfg.Storage.AllowedBackends[0] == "file" {
		return storage.CleanLegacyEncryptedFile(filepath.Join(os.TempDir(), ".keyring.jwt"))
	}
	return storage.ErrStoreUnavailable
}
