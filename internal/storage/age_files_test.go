package storage

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/napisani/secret_inject/internal/secret"
)

func ageFixture(t *testing.T) *ageFiles {
	t.Helper()
	native, err := openAgeFiles(t.Context(), filepath.Join(t.TempDir(), "age"), InteractiveSetup)
	if err != nil {
		t.Fatal(err)
	}
	return native.(*ageFiles)
}

func TestAgeFilesPreparationAndEncryptedTransport(t *testing.T) {
	t.Parallel()
	directory := filepath.Join(t.TempDir(), "age")
	if _, err := openAgeFiles(t.Context(), directory, Unattended); !errors.Is(err, ErrInteractionRequired) {
		t.Fatal("unprepared unattended store was accepted")
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("unattended open created a directory")
	}
	native, err := openAgeFiles(t.Context(), directory, InteractiveSetup)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(directory, "identity"))
	if err != nil {
		t.Fatal(err)
	}
	for _, policy := range []StorePolicy{InteractiveSetup, Unattended} {
		if _, err := openAgeFiles(t.Context(), directory, policy); err != nil {
			t.Fatal(err)
		}
	}
	after, err := os.ReadFile(filepath.Join(directory, "identity"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("opening a prepared store replaced its identity")
	}
	for _, path := range []string{directory, filepath.Join(directory, "records"), filepath.Join(directory, "identity")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		want := os.FileMode(0600)
		if info.IsDir() {
			want = 0700
		}
		if info.Mode().Perm() != want {
			t.Fatal("store path is not private")
		}
	}
	key := "bootstrap-v1/synthetic"
	value := []byte("SYNTHETIC_PRIVATE_VALUE")
	if err := native.Put(t.Context(), key, value); err != nil {
		t.Fatal(err)
	}
	name, _ := encryptedObjectName(key, ".age")
	path := filepath.Join(directory, "records", name)
	cipher, err := os.ReadFile(path)
	if err != nil || bytes.Contains(cipher, value) || !bytes.HasPrefix(cipher, []byte("age-encryption.org/v1\n")) {
		t.Fatal("record is not an age-encrypted file")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("encrypted record is not private")
	}
	read, found, err := native.Get(t.Context(), key)
	if err != nil || !found || !bytes.Equal(read, value) {
		t.Fatal("encrypted roundtrip failed")
	}
	if err := native.Put(t.Context(), key, []byte("SYNTHETIC_REPLACEMENT")); err != nil {
		t.Fatal(err)
	}
	read, found, err = native.Get(t.Context(), key)
	if err != nil || !found || string(read) != "SYNTHETIC_REPLACEMENT" {
		t.Fatal("upsert failed")
	}
	keys, err := native.Keys(t.Context())
	if err != nil || !slices.Equal(keys, []string{key}) {
		t.Fatal("record enumeration failed")
	}
	for range 2 {
		if err := native.Delete(t.Context(), key); err != nil {
			t.Fatal("deletion failed or was not idempotent")
		}
	}
	if _, found, err := native.Get(t.Context(), key); err != nil || found {
		t.Fatal("deleted record survived")
	}
}

func TestAgeFilesRejectCiphertextDamageAndRenaming(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"rename", "tamper", "truncate", "oversized", "plaintext", "other identity"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			native := ageFixture(t)
			key := "bootstrap-v1/synthetic"
			if err := native.Put(t.Context(), key, []byte("SYNTHETIC_PRIVATE_VALUE")); err != nil {
				t.Fatal(err)
			}
			name, _ := encryptedObjectName(key, ".age")
			path := filepath.Join(native.directory, name)
			cipher, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "rename":
				key = "bootstrap-v1/other"
				name, _ = encryptedObjectName(key, ".age")
				path = filepath.Join(native.directory, name)
			case "tamper":
				cipher[len(cipher)-1] ^= 1
			case "truncate":
				cipher = cipher[:len(cipher)-1]
			case "oversized":
				cipher = make([]byte, ageCipherLimit+1)
			case "plaintext":
				cipher = []byte("PRIVATE_ERROR_SENTINEL")
			case "other identity":
				other := ageFixture(t)
				cipher, err = other.encrypt(t.Context(), name, []byte("SYNTHETIC_PRIVATE_VALUE"))
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(path, cipher, 0600); err != nil {
				t.Fatal(err)
			}
			value, found, err := native.Get(t.Context(), key)
			if err == nil || found || value != nil || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("invalid ciphertext was accepted or leaked details")
			}
		})
	}
}

func TestAgeFilesMissingOrInvalidIdentityFailsClosed(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"missing", "invalid", "oversized", "symlink", "replacement"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			native := ageFixture(t)
			if err := native.Put(t.Context(), "synthetic", []byte("SYNTHETIC")); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(native.identityPath); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "invalid":
				if err := os.WriteFile(native.identityPath, []byte("PRIVATE_IDENTITY_SENTINEL"), 0600); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				if err := os.WriteFile(native.identityPath, make([]byte, ageIdentityLimit+1), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				other := ageFixture(t)
				if err := os.Symlink(other.identityPath, native.identityPath); err != nil {
					t.Fatal(err)
				}
			case "replacement":
				other := ageFixture(t)
				data, err := os.ReadFile(other.identityPath)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(native.identityPath, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := native.Get(t.Context(), "synthetic"); err == nil || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("an existing handle ignored identity loss or replacement")
			}
			if err := native.Put(t.Context(), "new", []byte("SYNTHETIC")); err == nil {
				t.Fatal("ordinary write ignored identity loss or replacement")
			}
			for _, policy := range []StorePolicy{Unattended, InteractiveSetup} {
				opened, err := openAgeFiles(t.Context(), filepath.Dir(native.identityPath), policy)
				if err == nil {
					if mode != "replacement" {
						t.Fatal("opening repaired an invalid or missing identity")
					}
					if _, _, err := opened.Get(t.Context(), "synthetic"); err == nil {
						t.Fatal("replacement identity decrypted existing records")
					}
				}
			}
			if mode == "missing" {
				if _, err := os.Lstat(native.identityPath); !os.IsNotExist(err) {
					t.Fatal("preparation silently regenerated a lost identity")
				}
			}
		})
	}
}

func TestAgeFilesUseSharedRecordEngine(t *testing.T) {
	t.Parallel()
	native := ageFixture(t)
	directory := filepath.Dir(native.identityPath)
	lock := NewStoreFileLock(filepath.Join(t.TempDir(), "locks"), "synthetic-age-store")
	store, err := NewRecordStore(native, lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetCredential(t.Context(), "synthetic-root", "doppler", "SYNTHETIC_BOOTSTRAP"); err != nil {
		t.Fatal(err)
	}
	credentials, err := store.AcquireCredentials(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := secret.New()
	snapshot.Entries["SYNTHETIC_WORKLOAD"] = strings.Repeat("synthetic-value-", 12000)
	scope := strings.Repeat("a", 64)
	if err := store.WriteSnapshot(t.Context(), scope, credentials.Revision, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	nativeReopened, err := openAgeFiles(t.Context(), directory, Unattended)
	if err != nil {
		t.Fatal(err)
	}
	store, err = NewRecordStore(nativeReopened, lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	cached, found, err := store.ReadSnapshot(t.Context(), scope, time.Hour)
	if err != nil || !found || cached.Secrets.Entries["SYNTHETIC_WORKLOAD"] != snapshot.Entries["SYNTHETIC_WORKLOAD"] {
		t.Fatal("reopened store lost the multi-chunk workload snapshot")
	}
	if err := store.PrepareAccess(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.CleanSnapshots(t.Context()); err != nil {
		t.Fatal(err)
	}
	if value, err := store.ReadCredential(t.Context(), "synthetic-root", "doppler"); err != nil || value != "SYNTHETIC_BOOTSTRAP" {
		t.Fatal("cache cleanup damaged the credential")
	}
	if err := store.SetCredential(t.Context(), "synthetic-root", "doppler", "SYNTHETIC_UPDATED"); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckRevision(t.Context(), credentials.Revision); !errors.Is(err, ErrCredentialContextChanged) {
		t.Fatal("credential mutation did not fence the old revision")
	}
}

func TestAgeFilesHonorCancellationAndLimits(t *testing.T) {
	t.Parallel()
	native := ageFixture(t)
	if err := native.Put(t.Context(), "too-large", make([]byte, ageValueLimit+1)); !errors.Is(err, ErrStoreOperation) {
		t.Fatal("oversized plaintext was accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := openAgeFiles(ctx, filepath.Join(t.TempDir(), "must-not-create"), InteractiveSetup); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled preparation was accepted")
	}
	if _, _, err := native.Get(ctx, "synthetic"); !errors.Is(err, context.Canceled) {
		t.Fatal("read ignored cancellation")
	}
	if err := native.Put(ctx, "synthetic", []byte("SYNTHETIC")); !errors.Is(err, context.Canceled) {
		t.Fatal("write ignored cancellation")
	}
	if err := native.Delete(ctx, "synthetic"); !errors.Is(err, context.Canceled) {
		t.Fatal("delete ignored cancellation")
	}
	if _, err := native.Keys(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("enumeration ignored cancellation")
	}
}
