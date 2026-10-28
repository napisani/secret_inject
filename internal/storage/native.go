package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"time"

	"github.com/napisani/secret_inject/internal/config"
	"github.com/napisani/secret_inject/internal/privatefs"
)

type StorePolicy uint8

const (
	Unattended StorePolicy = iota
	InteractiveSetup
)

func nativeBackend(cfg *config.StorageConfig, system string) (string, error) {
	if cfg == nil {
		return "", ErrStoreUnavailable
	}
	if cfg.Type != "keyring" || cfg.Password != "" {
		return "", ErrStoreUnavailable
	}
	platforms := map[string]string{
		"darwin-keychain": "darwin", "linux-secret-service": "linux",
		"linux-systemd-credentials": "linux",
	}
	selected := map[string]string{}
	for _, backend := range cfg.AllowedBackends {
		platform, known := platforms[backend]
		if !known || selected[platform] != "" {
			return "", ErrStoreUnavailable
		}
		selected[platform] = backend
	}
	if selected[system] == "" {
		return "", ErrStoreUnavailable
	}
	return selected[system], nil
}

func ValidateNativeConfig(cfg *config.StorageConfig) error {
	_, err := nativeBackend(cfg, runtime.GOOS)
	return err
}

func OpenNative(ctx context.Context, cfg *config.StorageConfig, policy StorePolicy) (*RecordStore, error) {
	backend, err := nativeBackend(cfg, runtime.GOOS)
	if err != nil {
		return nil, err
	}
	if policy != Unattended && policy != InteractiveSetup {
		return nil, ErrStoreUnavailable
	}
	account, err := user.Current()
	if err != nil || !filepath.IsAbs(account.HomeDir) {
		return nil, ErrStoreUnavailable
	}
	privateRoot := filepath.Join(account.HomeDir, ".secret_inject")
	if err := privatefs.EnsureDir(privateRoot); err != nil {
		return nil, ErrStoreUnavailable
	}
	lock := NewStoreFileLock(filepath.Join(privateRoot, "locks"), backend+"/secret_inject")
	// Container preparation participates in the same store-wide lock as CRUD.
	release, err := lock.Lock(ctx)
	if err != nil {
		return nil, err
	}
	var native NativeByteAdapter
	var openErr error
	if backend == "linux-systemd-credentials" {
		native, openErr = openSystemdCredentials(ctx, filepath.Join(privateRoot, "systemd-credentials"), policy)
	} else {
		native, openErr = openNativeAdapter(ctx, policy)
	}
	err = errors.Join(openErr, release())
	if err != nil {
		if native != nil {
			err = errors.Join(err, native.Close())
		}
		return nil, safeStoreFailure(err)
	}
	store, err := NewRecordStore(native, lock)
	if err != nil {
		return nil, errors.Join(err, safeStoreFailure(native.Close()))
	}
	return store, nil
}

type StoreFileLock struct{ directory, name string }

func NewStoreFileLock(directory, identity string) *StoreFileLock {
	digest := sha256.Sum256([]byte(identity))
	return &StoreFileLock{directory: directory, name: hex.EncodeToString(digest[:]) + ".lock"}
}

func (l *StoreFileLock) Lock(ctx context.Context) (func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := privatefs.EnsureDir(l.directory); err != nil {
		return nil, ErrStoreUnavailable
	}
	file, err := privatefs.Open(filepath.Join(l.directory, l.name), os.O_CREATE|os.O_RDWR)
	if err != nil {
		return nil, ErrStoreUnavailable
	}
	for {
		err = privatefs.TryLock(file)
		if err == nil {
			return func() error { return safeStoreFailure(errors.Join(privatefs.Unlock(file), file.Close())) }, nil
		}
		if !privatefs.LockBusy(err) {
			return nil, errors.Join(ErrStoreOperation, safeStoreFailure(file.Close()))
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, errors.Join(ctx.Err(), safeStoreFailure(file.Close()))
		case <-timer.C:
		}
	}
}
