package main

import (
	"context"
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/napisani/secret_inject/internal/config"
	"github.com/napisani/secret_inject/internal/storage"
)

type approvalNative struct {
	storage.NativeByteAdapter
	interactive bool
	approved    map[string]bool
}

func (n *approvalNative) Get(ctx context.Context, key string) ([]byte, bool, error) {
	value, found, err := n.NativeByteAdapter.Get(ctx, key)
	if err != nil || !found {
		return value, found, err
	}
	if !n.approved[key] {
		if !n.interactive {
			return nil, false, storage.ErrInteractionRequired
		}
		n.approved[key] = true
	}
	return value, true, nil
}

func TestStorePreparationAuthorizesExistingNativeItems(t *testing.T) {
	path, _ := commandFixture(t, true)
	native := &fixtureNative{values: make(map[string][]byte)}
	lock := &fixtureLock{}
	seed, err := storage.NewRecordStore(native, lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.SetCredential(t.Context(), "work-root", "doppler", "ROOT_SENTINEL"); err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	previous := openProtectedStore
	t.Cleanup(func() { openProtectedStore = previous })
	candidate := true
	allowApproval := false
	approved := make(map[string]bool)
	openProtectedStore = func(ctx context.Context, cfg *config.StorageConfig, policy storage.StorePolicy) (*storage.RecordStore, error) {
		if candidate {
			return storage.NewRecordStore(native, lock)
		}
		return storage.NewRecordStore(&approvalNative{
			NativeByteAdapter: native,
			interactive:       policy == storage.InteractiveSetup && allowApproval,
			approved:          approved,
		}, lock)
	}
	if code, _, stderr := invoke("--config", path, "--profile", "shell"); code != 0 {
		t.Fatalf("candidate warm-up failed: %s", stderr)
	}
	if err := native.Put(t.Context(), "secret_inject", []byte("OBSOLETE_PRIVATE_PAYLOAD_SENTINEL")); err != nil {
		t.Fatal(err)
	}
	before := maps.Clone(native.values)
	candidate = false
	t.Setenv("PATH", t.TempDir())
	if code, _, stderr := invoke("--config", path, "--profile", "shell"); code != 1 || !strings.Contains(stderr, "explicit preparation") {
		t.Fatal("fixture did not reproduce the installed-binary access denial")
	}
	if code, out, stderr := invoke("store", "prepare", "--config", path); code != 1 || out != "" || !strings.Contains(stderr, `"interaction_required"`) {
		t.Fatal("denied native approval returned a successful preparation receipt")
	}
	allowApproval = true
	code, out, stderr := invoke("store", "prepare", "--config", path)
	if code != 0 || stderr != "" {
		t.Fatalf("installed-binary preparation failed: %s", stderr)
	}
	assertJSONResponse(t, out, `{"schema_version":1,"store":{"kind":"prepared"}}`)
	if approved["secret_inject"] {
		t.Fatal("preparation accessed an obsolete payload")
	}
	if code, _, stderr := invoke("--config", path, "--profile", "shell"); code != 0 {
		t.Fatalf("prepared installed binary cannot read the existing cache: %s", stderr)
	}
	if !reflect.DeepEqual(native.values, before) {
		t.Fatal("native-item preparation changed protected records or cache eligibility")
	}
}
