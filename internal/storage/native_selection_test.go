package storage

import (
	"errors"
	"testing"

	"github.com/napisani/secret_inject/internal/config"
)

func TestNativeBackendSelectionUsesSystemNotListOrder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		system   string
		backends []string
		want     string
	}{
		{
			name: "macOS shared config", system: "darwin",
			backends: []string{"darwin-keychain", "linux-secret-service"}, want: "darwin-keychain",
		},
		{
			name: "Linux shared config", system: "linux",
			backends: []string{"darwin-keychain", "linux-secret-service"}, want: "linux-secret-service",
		},
		{
			name: "macOS reversed config", system: "darwin",
			backends: []string{"linux-secret-service", "darwin-keychain"}, want: "darwin-keychain",
		},
		{
			name: "Linux reversed config", system: "linux",
			backends: []string{"linux-secret-service", "darwin-keychain"}, want: "linux-secret-service",
		},
		{
			name: "macOS explicit config", system: "darwin",
			backends: []string{"darwin-keychain"}, want: "darwin-keychain",
		},
		{
			name: "Linux explicit config", system: "linux",
			backends: []string{"linux-secret-service"}, want: "linux-secret-service",
		},
		{
			name: "headless Linux", system: "linux",
			backends: []string{"darwin-keychain", "linux-systemd-credentials"}, want: "linux-systemd-credentials",
		},
		{
			name: "Mac keeps Keychain", system: "darwin",
			backends: []string{"linux-systemd-credentials", "darwin-keychain"}, want: "darwin-keychain",
		},
		{
			name: "ambiguous Linux", system: "linux",
			backends: []string{"linux-secret-service", "linux-systemd-credentials"},
		},
		{
			name: "macOS missing host backend", system: "darwin",
			backends: []string{"linux-secret-service"},
		},
		{
			name: "Linux missing host backend", system: "linux",
			backends: []string{"darwin-keychain"},
		},
		{
			name: "unsupported system", system: "freebsd",
			backends: []string{"darwin-keychain", "linux-secret-service"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &config.StorageConfig{Type: "keyring", AllowedBackends: tt.backends}
			got, err := nativeBackend(cfg, tt.system)
			if tt.want == "" {
				if !errors.Is(err, ErrStoreUnavailable) || got != "" {
					t.Fatal("unsupported system configuration returned a backend")
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("selected %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}
