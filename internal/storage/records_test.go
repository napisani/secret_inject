package storage_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/napisani/secret_inject/internal/secret"

	"github.com/napisani/secret_inject/internal/storage"
)

func TestRecordStoreKeepsCredentialValuesOutOfMetadata(t *testing.T) {
	t.Parallel()
	native := newMemoryNative(512)
	store, err := storage.NewRecordStore(native, newMemoryStoreLock())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := t.Context()
	if err := store.SetCredential(ctx, "work-root", "doppler", "PRIVATE_TOKEN_SENTINEL"); err != nil {
		t.Fatal(err)
	}
	value, err := store.ReadCredential(ctx, "work-root", "doppler")
	if err != nil || value != "PRIVATE_TOKEN_SENTINEL" {
		t.Fatalf("credential read: value matched=%t err=%v", value == "PRIVATE_TOKEN_SENTINEL", err)
	}
	metadata, err := store.ListCredentials(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []storage.CredentialMetadata{{Name: "work-root", Provider: "doppler"}}
	if !reflect.DeepEqual(metadata, want) {
		t.Fatalf("metadata=%#v, want %#v", metadata, want)
	}
}

func TestRecordStoreCredentialChangesInvalidateAllSnapshots(t *testing.T) {
	t.Parallel()
	store, err := storage.NewRecordStore(newMemoryNative(512), newMemoryStoreLock())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := t.Context()
	if err := store.SetCredential(ctx, "work-root", "doppler", "FIRST_PRIVATE_TOKEN"); err != nil {
		t.Fatal(err)
	}
	credentials, err := store.AcquireCredentials(ctx, []storage.CredentialRequest{{Name: "work-root", Provider: "doppler"}})
	if err != nil {
		t.Fatal(err)
	}
	value, err := credentials.Value("work-root", "doppler")
	if err != nil || value != "FIRST_PRIVATE_TOKEN" {
		t.Fatal("credential acquisition did not preserve its private value")
	}
	snapshot := secret.New()
	snapshot.Entries["WORKLOAD_KEY"] = "WORKLOAD_VALUE"
	for _, scope := range []string{strings.Repeat("a", 64), strings.Repeat("b", 64)} {
		if err := store.WriteSnapshot(ctx, scope, credentials.Revision, snapshot); err != nil {
			t.Fatal(err)
		}
		cached, found, err := store.ReadSnapshot(ctx, scope, time.Hour)
		if err != nil || !found || cached.Revision != credentials.Revision || !reflect.DeepEqual(cached.Secrets.Entries, snapshot.Entries) {
			t.Fatalf("snapshot hit=%t err=%v", found, err)
		}
	}
	if err := store.SetCredential(ctx, "work-root", "doppler", "SECOND_PRIVATE_TOKEN"); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{strings.Repeat("a", 64), strings.Repeat("b", 64)} {
		if _, found, err := store.ReadSnapshot(ctx, scope, time.Hour); err != nil || found {
			t.Fatalf("credential mutation left eligible snapshot: hit=%t err=%v", found, err)
		}
		if err := store.WriteSnapshot(ctx, scope, credentials.Revision, snapshot); !errors.Is(err, storage.ErrCredentialContextChanged) {
			t.Fatalf("stale publication was not fenced: %v", err)
		}
	}
	if err := store.CheckRevision(ctx, credentials.Revision); !errors.Is(err, storage.ErrCredentialContextChanged) {
		t.Fatalf("stale final handoff was not fenced: %v", err)
	}
}

func TestRecordStoreSnapshotCleanupPreservesCredentialRoles(t *testing.T) {
	t.Parallel()
	store, native := recordStoreFixture(t, 512)
	ctx := t.Context()
	if err := store.SetCredential(ctx, "work-root", "doppler", strings.Repeat("PRIVATE_TOKEN_", 100)); err != nil {
		t.Fatal(err)
	}
	credentials, err := store.AcquireCredentials(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	scope := strings.Repeat("a", 64)
	snapshot := secret.New()
	snapshot.Entries["WORKLOAD_KEY"] = strings.Repeat("WORKLOAD_VALUE_", 100)
	if err := store.WriteSnapshot(ctx, scope, credentials.Revision, snapshot); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"secret_inject", "sources-v1-" + scope} {
		if err := native.Put(ctx, key, []byte("MALFORMED_LEGACY_SNAPSHOT")); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.CleanSnapshots(ctx); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.ReadSnapshot(ctx, scope, time.Hour); err != nil || found {
		t.Fatalf("cleaned snapshot hit=%t err=%v", found, err)
	}
	value, err := store.ReadCredential(ctx, "work-root", "doppler")
	if err != nil || value != strings.Repeat("PRIVATE_TOKEN_", 100) {
		t.Fatal("snapshot cleanup damaged a credential")
	}
	if err := store.CheckRevision(ctx, credentials.Revision); err != nil {
		t.Fatal("snapshot cleanup changed the credential revision")
	}
	keys, err := native.Keys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if strings.HasPrefix(key, "snapshot-v2/") || strings.HasPrefix(key, "object-chunks/snapshot-v2/") ||
			strings.HasPrefix(key, "sources-v1-") || key == "secret_inject" {
			t.Fatal("snapshot cleanup left an owned workload entry")
		}
	}
}

func TestRecordStoreRemovalInvalidatesSnapshotsButPreservesOtherCredentials(t *testing.T) {
	t.Parallel()
	store, _ := recordStoreFixture(t, 512)
	ctx := t.Context()
	if err := store.SetCredential(ctx, "work-root", "doppler", "PRIVATE_WORK_TOKEN"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetCredential(ctx, "other-root", "onepassword", "PRIVATE_OTHER_TOKEN"); err != nil {
		t.Fatal(err)
	}
	credentials, err := store.AcquireCredentials(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := secret.New()
	snapshot.Entries["KEY"] = "VALUE"
	scope := strings.Repeat("a", 64)
	if err := store.WriteSnapshot(ctx, scope, credentials.Revision, snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RemoveCredential(ctx, "work-root"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadCredential(ctx, "work-root", "doppler"); !errors.Is(err, storage.ErrCredentialNotFound) {
		t.Fatalf("removed credential remained available: %v", err)
	}
	value, err := store.ReadCredential(ctx, "other-root", "onepassword")
	if err != nil || value != "PRIVATE_OTHER_TOKEN" {
		t.Fatal("removal damaged another credential")
	}
	if _, found, err := store.ReadSnapshot(ctx, scope, time.Hour); err != nil || found {
		t.Fatalf("removal left a snapshot eligible: hit=%t err=%v", found, err)
	}
	if err := store.CheckRevision(ctx, credentials.Revision); !errors.Is(err, storage.ErrCredentialContextChanged) {
		t.Fatal("removal did not fence an earlier credential context")
	}
}

func TestRecordStoreFailedCredentialMutationsStillFenceSnapshots(t *testing.T) {
	t.Parallel()
	for _, committed := range []bool{false, true} {
		name := "before commit"
		if committed {
			name = "after uncertain commit"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store, native := recordStoreFixture(t, 512)
			ctx := t.Context()
			if err := store.SetCredential(ctx, "work-root", "doppler", "OLD_PRIVATE_TOKEN"); err != nil {
				t.Fatal(err)
			}
			credentials, err := store.AcquireCredentials(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			snapshot := secret.New()
			snapshot.Entries["KEY"] = "VALUE"
			scope := strings.Repeat("a", 64)
			if err := store.WriteSnapshot(ctx, scope, credentials.Revision, snapshot); err != nil {
				t.Fatal(err)
			}
			attempts := 0
			fail := func(key string) error {
				if key == "bootstrap-v1/work-root" {
					attempts++
					return errors.New("PRIVATE_NATIVE_ERROR_SENTINEL")
				}
				return nil
			}
			if committed {
				native.afterPut = fail
			} else {
				native.before = func(operation, key string) error {
					if operation == "put" {
						return fail(key)
					}
					return nil
				}
			}
			err = store.SetCredential(ctx, "work-root", "doppler", "NEW_PRIVATE_TOKEN")
			if !errors.Is(err, storage.ErrStoreOperation) || attempts != 1 || strings.Contains(err.Error(), "SENTINEL") {
				t.Fatalf("unsafe mutation failure: attempts=%d err=%v", attempts, err)
			}
			native.before, native.afterPut = nil, nil
			value, err := store.ReadCredential(ctx, "work-root", "doppler")
			want := "OLD_PRIVATE_TOKEN"
			if committed {
				want = "NEW_PRIVATE_TOKEN"
			}
			if err != nil || value != want {
				t.Fatal("uncertain publication destroyed a valid credential object")
			}
			if _, found, err := store.ReadSnapshot(ctx, scope, time.Hour); err != nil || found {
				t.Fatalf("failed credential mutation left cache eligible: found=%t err=%v", found, err)
			}
			if err := store.CheckRevision(ctx, credentials.Revision); !errors.Is(err, storage.ErrCredentialContextChanged) {
				t.Fatal("failed credential mutation did not advance the fence")
			}
		})
	}
}

func TestRecordStoreRejectsProviderReplacementWithoutChangingRevision(t *testing.T) {
	t.Parallel()
	store, _ := recordStoreFixture(t, 512)
	ctx := t.Context()
	if err := store.SetCredential(ctx, "work-root", "doppler", "PRIVATE_TOKEN"); err != nil {
		t.Fatal(err)
	}
	credentials, err := store.AcquireCredentials(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetCredential(ctx, "work-root", "onepassword", "OTHER_PRIVATE_TOKEN"); !errors.Is(err, storage.ErrCredentialProviderMismatch) {
		t.Fatalf("provider replacement was accepted: %v", err)
	}
	if err := store.CheckRevision(ctx, credentials.Revision); err != nil {
		t.Fatal("rejected provider replacement invalidated snapshots")
	}
	if _, err := store.ReadCredential(ctx, "work-root", "onepassword"); !errors.Is(err, storage.ErrCredentialProviderMismatch) {
		t.Fatal("a credential was supplied to the wrong provider")
	}
}

func TestRecordStoreAcquiresNoPartialPrivateContext(t *testing.T) {
	t.Parallel()
	store, _ := recordStoreFixture(t, 512)
	ctx := t.Context()
	if err := store.SetCredential(ctx, "work-root", "doppler", "PRIVATE_TOKEN"); err != nil {
		t.Fatal(err)
	}
	credentials, err := store.AcquireCredentials(ctx, []storage.CredentialRequest{
		{Name: "work-root", Provider: "doppler"},
		{Name: "missing", Provider: "onepassword"},
	})
	if !errors.Is(err, storage.ErrCredentialNotFound) || credentials.Revision != "" {
		t.Fatal("failed acquisition returned a partial context")
	}
	if value, err := credentials.Value("work-root", "doppler"); err == nil || value != "" {
		t.Fatal("failed acquisition retained an accessible credential")
	}
}

func TestRecordStoreFencesAcrossHandles(t *testing.T) {
	t.Parallel()
	native := newMemoryNative(512)
	lock := newMemoryStoreLock()
	first, err := storage.NewRecordStore(native, lock)
	if err != nil {
		t.Fatal(err)
	}
	second, err := storage.NewRecordStore(native, lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := errors.Join(first.Close(), second.Close()); err != nil {
			t.Error(err)
		}
	})
	ctx := t.Context()
	credentials, err := first.AcquireCredentials(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.SetCredential(ctx, "work-root", "doppler", "PRIVATE_TOKEN"); err != nil {
		t.Fatal(err)
	}
	snapshot := secret.New()
	if err := first.WriteSnapshot(ctx, strings.Repeat("a", 64), credentials.Revision, snapshot); !errors.Is(err, storage.ErrCredentialContextChanged) {
		t.Fatalf("a different handle bypassed publication fencing: %v", err)
	}
}

func TestRecordStoreCacheHitsDoNotReadBootstrapValues(t *testing.T) {
	t.Parallel()
	store, native := recordStoreFixture(t, 512)
	ctx := t.Context()
	if err := store.SetCredential(ctx, "work-root", "doppler", "PRIVATE_TOKEN"); err != nil {
		t.Fatal(err)
	}
	credentials, err := store.AcquireCredentials(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := secret.New()
	snapshot.Entries["KEY"] = "VALUE"
	scope := strings.Repeat("a", 64)
	if err := store.WriteSnapshot(ctx, scope, credentials.Revision, snapshot); err != nil {
		t.Fatal(err)
	}
	native.before = func(operation, key string) error {
		if operation == "get" && (strings.HasPrefix(key, "bootstrap-v1/") || strings.HasPrefix(key, "object-chunks/bootstrap-v1/")) {
			return storage.ErrInteractionRequired
		}
		return nil
	}
	cached, found, err := store.ReadSnapshot(ctx, scope, time.Hour)
	if err != nil || !found || cached.Secrets.Entries["KEY"] != "VALUE" {
		t.Fatalf("cache hit required bootstrap access: found=%t err=%v", found, err)
	}
}

func TestRecordStoreFormatInvalidationPreservesCredentialsAndGeneration(t *testing.T) {
	t.Parallel()
	store, native := recordStoreFixture(t, 512)
	ctx := t.Context()
	if err := store.SetCredential(ctx, "work-root", "doppler", "PRIVATE_TOKEN"); err != nil {
		t.Fatal(err)
	}
	credentials, err := store.AcquireCredentials(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	scope := strings.Repeat("a", 64)
	if err := store.WriteSnapshot(ctx, scope, credentials.Revision, secret.New()); err != nil {
		t.Fatal(err)
	}
	if err := native.Put(ctx, "metadata-v2/snapshot-format", []byte("MALFORMED_FORMAT_SENTINEL")); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.ReadSnapshot(ctx, scope, time.Hour); err != nil || found {
		t.Fatalf("invalid snapshot format was reused: found=%t err=%v", found, err)
	}
	if err := store.CheckRevision(ctx, credentials.Revision); err != nil {
		t.Fatal("format invalidation changed credential generation")
	}
	if value, err := store.ReadCredential(ctx, "work-root", "doppler"); err != nil || value != "PRIVATE_TOKEN" {
		t.Fatal("format invalidation damaged credentials")
	}
}

func TestRecordStoreNativeFailuresNeverTriggerFormatInvalidation(t *testing.T) {
	t.Parallel()
	for _, reason := range []error{storage.ErrInteractionRequired, errors.New("PRIVATE_NATIVE_ERROR_SENTINEL")} {
		name := "unclassified native failure"
		if errors.Is(reason, storage.ErrInteractionRequired) {
			name = "interaction required"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store, native := recordStoreFixture(t, 512)
			ctx := t.Context()
			credentials, err := store.AcquireCredentials(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			scope := strings.Repeat("a", 64)
			if err := store.WriteSnapshot(ctx, scope, credentials.Revision, secret.New()); err != nil {
				t.Fatal(err)
			}
			writes, deletes := 0, 0
			native.before = func(operation, key string) error {
				switch operation {
				case "get":
					if key == "metadata-v2/snapshot-format" {
						return fmt.Errorf("PRIVATE_NATIVE_CONTEXT: %w", reason)
					}
				case "put":
					writes++
				case "delete":
					deletes++
				}
				return nil
			}
			_, found, err := store.ReadSnapshot(ctx, scope, time.Hour)
			if err == nil || found || writes != 0 || deletes != 0 || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatalf("unsafe native failure recovery: hit=%t writes=%d deletes=%d err=%v", found, writes, deletes, err)
			}
			if errors.Is(reason, storage.ErrInteractionRequired) && !errors.Is(err, reason) {
				t.Fatal("native interaction classification was lost")
			}
			native.before = nil
			if _, found, err := store.ReadSnapshot(ctx, scope, time.Hour); err != nil || !found {
				t.Fatal("native read failure destroyed an eligible snapshot")
			}
		})
	}
}

func TestRecordStoreExpiresSnapshotsWithoutCredentialReads(t *testing.T) {
	t.Parallel()
	store, _ := recordStoreFixture(t, 512)
	ctx := t.Context()
	credentials, err := store.AcquireCredentials(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := secret.New()
	snapshot.Timestamp = snapshot.Timestamp.Add(-2 * time.Hour)
	scope := strings.Repeat("a", 64)
	if err := store.WriteSnapshot(ctx, scope, credentials.Revision, snapshot); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.ReadSnapshot(ctx, scope, time.Hour); err != nil || found {
		t.Fatalf("expired snapshot hit=%t err=%v", found, err)
	}
}

func TestCredentialContextSerializationOmitsValues(t *testing.T) {
	t.Parallel()
	store, _ := recordStoreFixture(t, 512)
	ctx := t.Context()
	if err := store.SetCredential(ctx, "work-root", "doppler", "PRIVATE_TOKEN_SENTINEL"); err != nil {
		t.Fatal(err)
	}
	credentials, err := store.AcquireCredentials(ctx, []storage.CredentialRequest{{Name: "work-root", Provider: "doppler"}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(credentials)
	if err != nil || bytes.Contains(encoded, []byte("PRIVATE_TOKEN_SENTINEL")) {
		t.Fatal("credential context serialization exposed private values")
	}
}

func TestRecordStoreCancellationAfterNativeReadReturnsNoCredential(t *testing.T) {
	t.Parallel()
	store, native := recordStoreFixture(t, 512)
	if err := store.SetCredential(t.Context(), "work-root", "doppler", "PRIVATE_TOKEN"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	native.before = func(operation, key string) error {
		if operation == "get" && strings.HasPrefix(key, "object-chunks/bootstrap-v1/") {
			cancel()
		}
		return nil
	}
	value, err := store.ReadCredential(ctx, "work-root", "doppler")
	if !errors.Is(err, context.Canceled) || value != "" {
		t.Fatalf("canceled native read exposed a value: nonempty=%t err=%v", value != "", err)
	}
}

func TestRecordStoreCancellationWhileWaitingForStoreLock(t *testing.T) {
	t.Parallel()
	native := newMemoryNative(512)
	lock := newMemoryStoreLock()
	store, err := storage.NewRecordStore(native, lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	release, err := lock.Lock(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := release(); err != nil {
			t.Error(err)
		}
	}()
	attempted := make(chan struct{})
	lock.onLock = func() { close(attempted) }
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := store.AcquireCredentials(ctx, nil)
		done <- err
	}()
	<-attempted
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("blocked lock ignored cancellation: %v", err)
	}
	keys, err := native.Keys(t.Context())
	if err != nil || len(keys) != 0 {
		t.Fatal("canceled lock acquisition performed native writes")
	}
}

func TestRecordStoreSanitizesCloseFailuresAndClosesOnce(t *testing.T) {
	t.Parallel()
	native := newMemoryNative(512)
	store, err := storage.NewRecordStore(native, newMemoryStoreLock())
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	native.before = func(operation, key string) error {
		if operation == "close" {
			calls++
			return errors.New("PRIVATE_NATIVE_CLOSE_SENTINEL")
		}
		return nil
	}
	for range 2 {
		err := store.Close()
		if !errors.Is(err, storage.ErrStoreOperation) || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatalf("unsafe native close failure: %v", err)
		}
	}
	if calls != 1 {
		t.Fatal("native adapter closed more than once")
	}
	if _, err := store.ReadCredential(t.Context(), "work-root", "doppler"); !errors.Is(err, storage.ErrStoreUnavailable) {
		t.Fatal("closed store allowed a new operation")
	}
}

func TestRecordStoreDoesNotRewriteInvalidSnapshotText(t *testing.T) {
	t.Parallel()
	store, _ := recordStoreFixture(t, 512)
	credentials, err := store.AcquireCredentials(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"invalid key bytes", string([]byte{0xff}), "value"},
		{"invalid value bytes", "KEY", string([]byte{0xff})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := secret.New()
			snapshot.Entries[tt.key] = tt.value
			if err := store.WriteSnapshot(t.Context(), strings.Repeat("a", 64), credentials.Revision, snapshot); !errors.Is(err, storage.ErrInvalidSnapshot) {
				t.Fatalf("unrepresentable snapshot text was accepted: %v", err)
			}
		})
	}
}

func TestRecordStoreInvalidInputsDoNotTouchNativeStorage(t *testing.T) {
	t.Parallel()
	store, native := recordStoreFixture(t, 512)
	tests := []struct {
		name     string
		id       string
		provider string
		value    string
	}{
		{"invalid name", "../bad", "doppler", "token"},
		{"unknown provider", "work-root", "unknown", "token"},
		{"empty value", "work-root", "doppler", ""},
		{"NUL value", "work-root", "doppler", "bad\x00token"},
		{"non-UTF-8 value", "work-root", "doppler", string([]byte{0xff})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := store.SetCredential(t.Context(), tt.id, tt.provider, tt.value); !errors.Is(err, storage.ErrInvalidCredential) {
				t.Fatalf("invalid credential was accepted: %v", err)
			}
		})
	}
	keys, err := native.Keys(t.Context())
	if err != nil || len(keys) != 0 {
		t.Fatal("invalid credential input changed native storage")
	}
}

func recordStoreFixture(t *testing.T, limit int) (*storage.RecordStore, *memoryNative) {
	t.Helper()
	native := newMemoryNative(limit)
	store, err := storage.NewRecordStore(native, newMemoryStoreLock())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	return store, native
}

type memoryNative struct {
	mu       sync.Mutex
	values   map[string][]byte
	limit    int
	closed   bool
	before   func(operation, key string) error
	afterPut func(key string) error
}

func newMemoryNative(limit int) *memoryNative {
	return &memoryNative{values: make(map[string][]byte), limit: limit}
}

func (m *memoryNative) MaxValueBytes() int { return m.limit }

func (m *memoryNative) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if m.before != nil {
		if err := m.before("get", key); err != nil {
			return nil, false, err
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	value, found := m.values[key]
	return bytes.Clone(value), found, nil
}

func (m *memoryNative) Put(ctx context.Context, key string, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.before != nil {
		if err := m.before("put", key); err != nil {
			return err
		}
	}
	if len(value) > m.limit {
		return errors.New("synthetic native value limit exceeded")
	}
	m.mu.Lock()
	m.values[key] = bytes.Clone(value)
	m.mu.Unlock()
	if m.afterPut != nil {
		return m.afterPut(key)
	}
	return nil
}

func (m *memoryNative) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.before != nil {
		if err := m.before("delete", key); err != nil {
			return err
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, key)
	return nil
}

func (m *memoryNative) Keys(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.before != nil {
		if err := m.before("keys", ""); err != nil {
			return nil, err
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.values))
	for key := range m.values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func (m *memoryNative) Close() error {
	if m.before != nil {
		if err := m.before("close", ""); err != nil {
			return err
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

type memoryStoreLock struct {
	gate   chan struct{}
	onLock func()
}

func newMemoryStoreLock() *memoryStoreLock {
	lock := &memoryStoreLock{gate: make(chan struct{}, 1)}
	lock.gate <- struct{}{}
	return lock
}

func (m *memoryStoreLock) Lock(ctx context.Context) (func() error, error) {
	if m.onLock != nil {
		m.onLock()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-m.gate:
		return func() error {
			m.gate <- struct{}{}
			return nil
		}, nil
	}
}
