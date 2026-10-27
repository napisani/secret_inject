package storage_test

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/napisani/secret_inject/internal/config"
	"github.com/napisani/secret_inject/internal/storage"
)

func TestNativeConfigurationAcceptsHostOrPortableBackends(t *testing.T) {
	t.Parallel()
	backend := map[string]string{"darwin": "darwin-keychain", "linux": "linux-secret-service"}[runtime.GOOS]
	otherBackend := map[string]string{"darwin": "linux-secret-service", "linux": "darwin-keychain"}[runtime.GOOS]
	allowed := [][]string{
		{backend},
		{"darwin-keychain", "linux-secret-service"},
		{"linux-secret-service", "darwin-keychain"},
	}
	for _, backends := range allowed {
		t.Run(strings.Join(backends, ","), func(t *testing.T) {
			t.Parallel()
			cfg := &config.StorageConfig{Type: "keyring", AllowedBackends: backends}
			if err := storage.ValidateNativeConfig(cfg); err != nil {
				t.Fatal(err)
			}
		})
	}
	tests := []struct {
		name string
		cfg  *config.StorageConfig
	}{
		{"missing", nil},
		{"file", &config.StorageConfig{Type: "file"}},
		{"fallback", &config.StorageConfig{Type: "keyring", AllowedBackends: []string{backend, "file"}}},
		{"duplicate", &config.StorageConfig{Type: "keyring", AllowedBackends: []string{backend, backend}}},
		{"empty entry", &config.StorageConfig{Type: "keyring", AllowedBackends: []string{backend, ""}}},
		{"unknown entry", &config.StorageConfig{Type: "keyring", AllowedBackends: []string{backend, "unknown"}}},
		{"unqualified Keychain name", &config.StorageConfig{Type: "keyring", AllowedBackends: []string{"keychain"}}},
		{"unqualified Secret Service name", &config.StorageConfig{
			Type: "keyring", AllowedBackends: []string{"secret-service"},
		}},
		{"wrong host", &config.StorageConfig{Type: "keyring", AllowedBackends: []string{otherBackend}}},
		{"password", &config.StorageConfig{Type: "keyring", AllowedBackends: []string{backend}, Password: "PRIVATE_SENTINEL"}},
		{"implicit backend", &config.StorageConfig{Type: "keyring"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := storage.ValidateNativeConfig(tt.cfg); !errors.Is(err, storage.ErrStoreUnavailable) {
				t.Fatal("unsupported native configuration accepted")
			}
		})
	}
}

func TestStoreFileLockCoordinatesHandlesAndHonorsCancellation(t *testing.T) {
	t.Parallel()
	directory := filepath.Join(t.TempDir(), "locks")
	first := storage.NewStoreFileLock(directory, "same-user-native-store")
	second := storage.NewStoreFileLock(directory, "same-user-native-store")
	release, err := first.Lock(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if unlock, err := second.Lock(ctx); unlock != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("contended store lock ignored cancellation")
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	unlock, err := second.Lock(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
}
