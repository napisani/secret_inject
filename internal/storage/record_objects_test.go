package storage_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/napisani/secret_inject/internal/secret"
	"github.com/napisani/secret_inject/internal/storage"
)

func TestRecordStoreReclaimsUnpublishedSnapshotChunksWithoutTouchingCredentials(t *testing.T) {
	t.Parallel()
	store, native := recordStoreFixture(t, 512)
	ctx := t.Context()
	if err := store.SetCredential(ctx, "work-root", "doppler", "PRIVATE_TOKEN_SENTINEL"); err != nil {
		t.Fatal(err)
	}
	credentials, err := store.AcquireCredentials(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	scope := strings.Repeat("a", 64)
	native.before = func(operation, key string) error {
		if operation == "put" && key == "snapshot-v2/"+scope {
			return errors.New("PRIVATE_NATIVE_ERROR_SENTINEL")
		}
		return nil
	}
	snapshot := secret.New()
	snapshot.Entries["WORKLOAD_KEY"] = strings.Repeat("WORKLOAD_VALUE_", 100)
	if err := store.WriteSnapshot(ctx, scope, credentials.Revision, snapshot); !errors.Is(err, storage.ErrStoreOperation) {
		t.Fatalf("manifest failure was not reported: %v", err)
	}
	native.before = nil
	if _, found, err := store.ReadSnapshot(ctx, scope, time.Hour); err != nil || found {
		t.Fatalf("unpublished snapshot became visible: found=%t err=%v", found, err)
	}
	if err := store.CleanSnapshots(ctx); err != nil {
		t.Fatal(err)
	}
	keys, err := native.Keys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if strings.HasPrefix(key, "object-chunks/snapshot-v2/") {
			t.Fatal("snapshot cleanup left orphan chunks")
		}
	}
	value, err := store.ReadCredential(ctx, "work-root", "doppler")
	if err != nil || value != "PRIVATE_TOKEN_SENTINEL" {
		t.Fatal("orphan cleanup damaged credentials")
	}
}

func TestRecordStoreRemovalReclaimsFailedCredentialEnrollmentChunks(t *testing.T) {
	t.Parallel()
	store, native := recordStoreFixture(t, 512)
	ctx := t.Context()
	native.before = func(operation, key string) error {
		if operation == "put" && key == "bootstrap-v1/work-root" {
			return errors.New("PRIVATE_NATIVE_ENROLLMENT_FAILURE")
		}
		return nil
	}
	if err := store.SetCredential(ctx, "work-root", "doppler", "PRIVATE_TOKEN_SENTINEL"); !errors.Is(err, storage.ErrStoreOperation) {
		t.Fatalf("enrollment failure was not reported: %v", err)
	}
	native.before = nil
	if _, err := store.RemoveCredential(ctx, "work-root"); !errors.Is(err, storage.ErrCredentialNotFound) {
		t.Fatalf("an unpublished credential was reported present: %v", err)
	}
	keys, err := native.Keys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if strings.HasPrefix(key, "object-chunks/bootstrap-v1/") {
			t.Fatal("removal left chunks from a failed credential enrollment")
		}
	}
}

func TestRecordStorePartialChunkFailurePreservesPreviousSnapshot(t *testing.T) {
	t.Parallel()
	store, native := recordStoreFixture(t, 512)
	ctx := t.Context()
	credentials, err := store.AcquireCredentials(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	scope := strings.Repeat("a", 64)
	previous := secret.New()
	previous.Entries["KEY"] = "PREVIOUS_VALUE"
	if err := store.WriteSnapshot(ctx, scope, credentials.Revision, previous); err != nil {
		t.Fatal(err)
	}
	before, err := native.Keys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	native.before = func(operation, key string) error {
		if operation == "put" && strings.HasPrefix(key, "object-chunks/snapshot-v2/") && strings.HasSuffix(key, "/1") {
			return errors.New("PRIVATE_NATIVE_CHUNK_FAILURE")
		}
		return nil
	}
	replacement := secret.New()
	replacement.Entries["KEY"] = strings.Repeat("REPLACEMENT_VALUE_", 100)
	if err := store.WriteSnapshot(ctx, scope, credentials.Revision, replacement); !errors.Is(err, storage.ErrStoreOperation) {
		t.Fatalf("partial chunk failure was not reported: %v", err)
	}
	native.before = nil
	cached, found, err := store.ReadSnapshot(ctx, scope, time.Hour)
	if err != nil || !found || cached.Secrets.Entries["KEY"] != "PREVIOUS_VALUE" {
		t.Fatal("partial chunk write damaged the previous snapshot")
	}
	after, err := native.Keys(ctx)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("failed chunk generation leaked native entries")
	}
}

func TestRecordStoreCanceledCredentialPublication(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		replace   bool
		uncertain bool
		manifest  bool
	}{
		{name: "new credential canceled between chunks"},
		{name: "replacement canceled between chunks", replace: true},
		{name: "new credential uncertain chunk commit", uncertain: true},
		{name: "replacement uncertain chunk commit", replace: true, uncertain: true},
		{name: "new credential uncertain manifest commit", manifest: true},
		{name: "replacement uncertain manifest commit", replace: true, manifest: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			native := newMemoryNative(512)
			adapter := &cleanupContextNative{NativeByteAdapter: native}
			store, err := storage.NewRecordStore(adapter, newMemoryStoreLock())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := store.Close(); err != nil {
					t.Error(err)
				}
			})
			ctx := t.Context()
			if err := store.SetCredential(ctx, "other-root", "bitwarden", "OTHER_PRIVATE_TOKEN"); err != nil {
				t.Fatal(err)
			}
			if tt.replace {
				if err := store.SetCredential(ctx, "work-root", "doppler", "OLD_PRIVATE_TOKEN"); err != nil {
					t.Fatal(err)
				}
			}
			credentials, err := store.AcquireCredentials(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			bootstrapKey := func(key string) bool {
				return strings.HasPrefix(key, "bootstrap-v1/") || strings.HasPrefix(key, "object-chunks/bootstrap-v1/")
			}
			before := map[string]bool{}
			for key := range native.values {
				if bootstrapKey(key) {
					before[key] = true
				}
			}
			cleanupCalls := 0
			adapter.inspectDelete = func(cleanupCtx context.Context, key string) {
				if !bootstrapKey(key) {
					return
				}
				cleanupCalls++
				if before[key] {
					t.Fatal("cleanup attempted to delete a previously published credential chunk")
				}
				if cleanupCtx.Err() != nil {
					t.Fatal("cleanup inherited request cancellation")
				}
				deadline, ok := cleanupCtx.Deadline()
				remaining := time.Until(deadline)
				if !ok || remaining <= 0 || remaining > 5*time.Second {
					t.Fatal("cleanup lacks a bounded independent deadline")
				}
			}
			request, cancel := context.WithCancel(ctx)
			defer cancel()
			canceled := false
			native.afterPut = func(key string) error {
				trigger := key == "bootstrap-v1/work-root"
				if !tt.manifest {
					trigger = strings.HasPrefix(key, "object-chunks/bootstrap-v1/") && strings.HasSuffix(key, "/0")
				}
				if trigger {
					canceled = true
					cancel()
					if tt.uncertain || tt.manifest {
						return request.Err()
					}
				}
				return nil
			}
			value := strings.Repeat("NEW_PRIVATE_TOKEN_", 100)
			if err := store.SetCredential(request, "work-root", "doppler", value); !errors.Is(err, context.Canceled) {
				t.Fatal("credential cancellation was not preserved")
			}
			native.afterPut = nil
			if !canceled {
				t.Fatal("fixture did not cancel credential publication")
			}
			if tt.manifest {
				if cleanupCalls != 0 {
					t.Fatal("uncertain manifest publication triggered chunk cleanup")
				}
				if got, err := store.ReadCredential(ctx, "work-root", "doppler"); err != nil || got != value {
					t.Fatal("uncertain manifest publication lost committed credential chunks")
				}
			} else {
				if cleanupCalls == 0 {
					t.Fatal("unpublished credential generation was not reclaimed")
				}
				after := map[string]bool{}
				for key := range native.values {
					if bootstrapKey(key) {
						after[key] = true
					}
				}
				if !reflect.DeepEqual(before, after) {
					t.Fatal("canceled credential write left chunks or removed unrelated entries")
				}
				got, err := store.ReadCredential(ctx, "work-root", "doppler")
				if tt.replace {
					if err != nil || got != "OLD_PRIVATE_TOKEN" {
						t.Fatal("canceled replacement damaged the previous credential")
					}
				} else if !errors.Is(err, storage.ErrCredentialNotFound) || got != "" {
					t.Fatal("canceled enrollment published a credential")
				}
			}
			if got, err := store.ReadCredential(ctx, "other-root", "bitwarden"); err != nil || got != "OTHER_PRIVATE_TOKEN" {
				t.Fatal("canceled publication damaged an unrelated credential")
			}
			if err := store.CheckRevision(ctx, credentials.Revision); !errors.Is(err, storage.ErrCredentialContextChanged) {
				t.Fatal("failed credential mutation did not preserve conservative cache invalidation")
			}
		})
	}
}

type cleanupContextNative struct {
	storage.NativeByteAdapter
	inspectDelete func(context.Context, string)
}

func (n *cleanupContextNative) Delete(ctx context.Context, key string) error {
	if n.inspectDelete != nil {
		n.inspectDelete(ctx, key)
	}
	return n.NativeByteAdapter.Delete(ctx, key)
}

func TestRecordStoreRejectsCorruptedNativeChunksWithoutReturningValues(t *testing.T) {
	t.Parallel()
	store, native := recordStoreFixture(t, 512)
	ctx := t.Context()
	if err := store.SetCredential(ctx, "work-root", "doppler", "PRIVATE_TOKEN_SENTINEL"); err != nil {
		t.Fatal(err)
	}
	keys, err := native.Keys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	corrupted := false
	for _, key := range keys {
		if strings.HasPrefix(key, "object-chunks/bootstrap-v1/") {
			data, found, err := native.Get(ctx, key)
			if err != nil || !found || len(data) == 0 {
				t.Fatal("synthetic credential chunk unavailable")
			}
			data[0] ^= 1
			if err := native.Put(ctx, key, data); err != nil {
				t.Fatal(err)
			}
			corrupted = true
			break
		}
	}
	if !corrupted {
		t.Fatal("native fixture did not contain a credential object")
	}
	value, err := store.ReadCredential(ctx, "work-root", "doppler")
	if value != "" || !errors.Is(err, storage.ErrRecordCorrupt) {
		t.Fatalf("corrupt object returned a value: nonempty=%t err=%v", value != "", err)
	}
	if metadata, err := store.ListCredentials(ctx); metadata != nil || !errors.Is(err, storage.ErrRecordCorrupt) {
		t.Fatal("corrupt credential produced metadata")
	}
}

func TestRecordStoreReadsPublishedChunkSizesAfterLimitChanges(t *testing.T) {
	t.Parallel()
	store, native := recordStoreFixture(t, 512)
	ctx := t.Context()
	value := strings.Repeat("PRIVATE_TOKEN_", 100)
	if err := store.SetCredential(ctx, "work-root", "doppler", value); err != nil {
		t.Fatal(err)
	}
	native.limit = 1024
	if got, err := store.ReadCredential(ctx, "work-root", "doppler"); err != nil || got != value {
		t.Fatal("native write limit change made a stored object unreadable")
	}
}

func FuzzRecordStoreMalformedNativeObjects(f *testing.F) {
	f.Add([]byte(`{"schema_version":1,"kind":"credential","name":"work-root","provider":"doppler","value":"PRIVATE_FUZZ_TOKEN"}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{"schema_version":1,"kind":"native_object","generation":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","chunk_size":0,"chunks":1,"byte_length":0,"sha256":"0000000000000000000000000000000000000000000000000000000000000000"}`))
	f.Add([]byte(`{"schema_version":1,"kind":"native_object","generation":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","chunk_size":512,"chunks":1,"byte_length":1,"sha256":"0000000000000000000000000000000000000000000000000000000000000000"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4096 {
			return
		}
		store, native := recordStoreFixture(t, 4096)
		if err := native.Put(t.Context(), "bootstrap-v1/work-root", data); err != nil {
			t.Fatal(err)
		}
		value, err := store.ReadCredential(t.Context(), "work-root", "doppler")
		if value != "" || !errors.Is(err, storage.ErrRecordCorrupt) {
			t.Fatal("a native object without its chunks returned a credential")
		}
	})
}

func TestRecordStoreChunksBothCredentialAndSnapshotValues(t *testing.T) {
	t.Parallel()
	store, _ := recordStoreFixture(t, 512)
	ctx := t.Context()
	privateValue := strings.Repeat("PRIVATE_TOKEN_SENTINEL_", 100)
	if err := store.SetCredential(ctx, "work-root", "doppler", privateValue); err != nil {
		t.Fatal(err)
	}
	value, err := store.ReadCredential(ctx, "work-root", "doppler")
	if err != nil || value != privateValue {
		t.Fatalf("large credential read matched=%t err=%v", value == privateValue, err)
	}
	credentials, err := store.AcquireCredentials(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := secret.New()
	snapshot.Entries["WORKLOAD_KEY"] = strings.Repeat("WORKLOAD_VALUE_", 200)
	scope := strings.Repeat("a", 64)
	if err := store.WriteSnapshot(ctx, scope, credentials.Revision, snapshot); err != nil {
		t.Fatal(err)
	}
	cached, found, err := store.ReadSnapshot(ctx, scope, time.Hour)
	if err != nil || !found || !reflect.DeepEqual(cached.Secrets.Entries, snapshot.Entries) {
		t.Fatalf("large snapshot hit=%t err=%v", found, err)
	}
}
