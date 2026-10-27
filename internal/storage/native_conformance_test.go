//go:build nativeconformance

package storage_test

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/napisani/secret_inject/internal/config"
	"github.com/napisani/secret_inject/internal/conformance"
	"github.com/napisani/secret_inject/internal/secret"
	"github.com/napisani/secret_inject/internal/storage"
)

func TestNativeConformancePreparedStoreRoundTripAndRoles(t *testing.T) {
	if err := conformance.RequireDisposableAccount(); err != nil {
		t.Fatal(err)
	}
	backend := map[string]string{"darwin": "darwin-keychain", "linux": "linux-secret-service"}[runtime.GOOS]
	store, err := storage.OpenNative(t.Context(), &config.StorageConfig{Type: "keyring", AllowedBackends: []string{backend}}, storage.Unattended)
	if err != nil {
		t.Fatal("prepared native store could not open unattended")
	}
	name := "native-conformance-fixture"
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := store.RemoveCredential(ctx, name); err != nil && !errors.Is(err, storage.ErrCredentialNotFound) {
			t.Error("native fixture removal failed")
		}
		if err := store.CleanSnapshots(ctx); err != nil {
			t.Error("native fixture snapshot cleanup failed")
		}
		if err := store.Close(); err != nil {
			t.Error("native fixture session close failed")
		}
	})
	value := strings.Repeat("synthetic-native-fixture-", 400)
	if err := store.SetCredential(t.Context(), name, "doppler", value); err != nil {
		t.Fatal("native bounded-size credential publication failed")
	}
	if got, err := store.ReadCredential(t.Context(), name, "doppler"); err != nil || got != value {
		t.Fatal("native credential roundtrip failed")
	}
	credentials, err := store.AcquireCredentials(t.Context(), []storage.CredentialRequest{{Name: name, Provider: "doppler"}})
	if err != nil {
		t.Fatal("native credential acquisition failed")
	}
	snapshot := secret.New()
	snapshot.Entries["SYNTHETIC_WORKLOAD"] = strings.Repeat("fixture", 1500)
	scope := strings.Repeat("a", 64)
	if err := store.WriteSnapshot(t.Context(), scope, credentials.Revision, snapshot); err != nil {
		t.Fatal("native bounded-size workload publication failed")
	}
	cached, found, err := store.ReadSnapshot(t.Context(), scope, time.Hour)
	if err != nil || !found || cached.Secrets.Entries["SYNTHETIC_WORKLOAD"] != snapshot.Entries["SYNTHETIC_WORKLOAD"] {
		t.Fatal("native workload roundtrip failed")
	}
	if err := store.CleanSnapshots(t.Context()); err != nil {
		t.Fatal("native workload cleanup failed")
	}
	if got, err := store.ReadCredential(t.Context(), name, "doppler"); err != nil || got != value {
		t.Fatal("native workload cleanup crossed credential ownership")
	}
	if err := store.SetCredential(t.Context(), name, "doppler", "synthetic-rotated-fixture"); err != nil {
		t.Fatal("native credential replacement failed")
	}
	if err := store.CheckRevision(t.Context(), credentials.Revision); !errors.Is(err, storage.ErrCredentialContextChanged) {
		t.Fatal("native credential mutation failed to fence prior context")
	}
}
