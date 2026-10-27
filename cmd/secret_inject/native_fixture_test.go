package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"runtime"
	"sync"
	"testing"

	"github.com/napisani/secret_inject/internal/config"
	"github.com/napisani/secret_inject/internal/source"
	"github.com/napisani/secret_inject/internal/storage"
)

type fixtureNative struct {
	mu     sync.Mutex
	values map[string][]byte
}

func (*fixtureNative) MaxValueBytes() int { return 2048 }
func (f *fixtureNative) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.values[key]
	return bytes.Clone(v), ok, nil
}
func (f *fixtureNative) Put(ctx context.Context, key string, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[key] = bytes.Clone(value)
	return nil
}
func (f *fixtureNative) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.values, key)
	return nil
}
func (f *fixtureNative) Keys(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	keys := make([]string, 0, len(f.values))
	for key := range f.values {
		keys = append(keys, key)
	}
	return keys, nil
}
func (*fixtureNative) Close() error { return nil }

type fixtureLock struct{ mu sync.Mutex }

func (l *fixtureLock) Lock(ctx context.Context) (func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	return func() error { l.mu.Unlock(); return nil }, nil
}

func fixtureBackend() string {
	return map[string]string{"darwin": "darwin-keychain", "linux": "linux-secret-service"}[runtime.GOOS]
}

func installProtectedFixture(t *testing.T) {
	t.Helper()
	native := &fixtureNative{values: make(map[string][]byte)}
	lock := &fixtureLock{}
	previousOpen, previousFetch := openProtectedStore, fetchRemoteProvider
	t.Cleanup(func() { openProtectedStore = previousOpen; fetchRemoteProvider = previousFetch })
	openProtectedStore = func(ctx context.Context, cfg *config.StorageConfig, policy storage.StorePolicy) (*storage.RecordStore, error) {
		return storage.NewRecordStore(native, lock)
	}
	store, err := storage.NewRecordStore(native, lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetCredential(t.Context(), "work-root", "doppler", "ROOT_SENTINEL"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	fetchRemoteProvider = func(ctx context.Context, provider string, raw json.RawMessage, token string) (map[string]string, error) {
		if os.Getenv("SI_PROVIDER_FAIL") == "1" {
			return nil, &source.ProviderError{Provider: provider, Operation: "fetch", Kind: "exit", ExitCode: 9}
		}
		if provider == "doppler" {
			if token != "ROOT_SENTINEL" {
				t.Fatal("wrong root token")
			}
			return map[string]string{"OP_SERVICE_ACCOUNT_TOKEN": "BOOTSTRAP_SENTINEL", "API_KEY": "DISCARDED_SENTINEL", "SERVICE_URL": "https://example.test"}, nil
		}
		if token != "BOOTSTRAP_SENTINEL" {
			t.Fatal("wrong derived token")
		}
		return map[string]string{"SERVICE_TOKEN": "FETCHED_SENTINEL"}, nil
	}
}
