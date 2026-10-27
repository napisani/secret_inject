package storage

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/napisani/secret_inject/internal/jsonobject"
	"github.com/napisani/secret_inject/internal/secret"
	"github.com/napisani/secret_inject/internal/source"
)

const (
	credentialPrefix      = "bootstrap-v1/"
	workloadPrefix        = "snapshot-v2/"
	credentialRevisionKey = "metadata-v2/credential-generation"
	snapshotFormatKey     = "metadata-v2/snapshot-format"
	snapshotFormat        = "2\n"
)

var (
	ErrStoreUnavailable           = errors.New("protected store unavailable")
	ErrStoreOperation             = errors.New("protected store operation failed")
	ErrInteractionRequired        = errors.New("protected store requires explicit preparation")
	ErrRecordCorrupt              = errors.New("invalid protected store record")
	ErrInvalidCredential          = errors.New("invalid credential metadata or value")
	ErrCredentialNotFound         = errors.New("credential unavailable")
	ErrCredentialProviderMismatch = errors.New("credential provider mismatch")
	ErrCredentialContextChanged   = errors.New("credential context changed; restart resolution")
	ErrInvalidSnapshot            = errors.New("invalid workload snapshot")
)

// Unattended NativeByteAdapter sessions never create/unlock containers or show UI.
// MaxValueBytes bounds writes; Delete is idempotent.
type NativeByteAdapter interface {
	MaxValueBytes() int
	Get(context.Context, string) ([]byte, bool, error)
	Put(context.Context, string, []byte) error
	Delete(context.Context, string) error
	Keys(context.Context) ([]string, error)
	Close() error
}

// StoreLock serializes all handles/processes using the same native store.
// A successful Lock returns a non-nil release function.
type StoreLock interface {
	Lock(context.Context) (func() error, error)
}

type CredentialMetadata struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

type CredentialRequest struct {
	Name     string
	Provider string
}

type CredentialContext struct {
	Revision string
	values   map[string]credentialRecord
}

func (c CredentialContext) Value(name, provider string) (string, error) {
	record, found := c.values[name]
	if !found {
		return "", ErrCredentialNotFound
	}
	if record.Provider != provider {
		return "", ErrCredentialProviderMismatch
	}
	return record.Value, nil
}

type WorkloadSnapshot struct {
	Secrets  *secret.Secrets
	Revision string
}

type workloadRecord struct {
	SchemaVersion int             `json:"schema_version"`
	Kind          string          `json:"kind"`
	Scope         string          `json:"scope"`
	Revision      string          `json:"credential_revision"`
	Snapshot      *secret.Secrets `json:"snapshot"`
}

type credentialRecord struct {
	SchemaVersion int    `json:"schema_version"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	Provider      string `json:"provider"`
	Value         string `json:"value"`
}

type RecordStore struct {
	native   NativeByteAdapter
	lock     StoreLock
	gate     chan struct{}
	closed   bool
	closeErr error
}

func NewRecordStore(native NativeByteAdapter, lock StoreLock) (*RecordStore, error) {
	if native == nil || lock == nil || native.MaxValueBytes() < 1 {
		return nil, ErrStoreUnavailable
	}
	store := &RecordStore{native: native, lock: lock, gate: make(chan struct{}, 1)}
	store.gate <- struct{}{}
	return store, nil
}

// PrepareAccess requests item approval for the current executable when supported.
// An unlocked native container does not establish access to its existing items.
func (s *RecordStore) PrepareAccess(ctx context.Context) error {
	return s.withLock(ctx, func() error {
		keys, err := s.native.Keys(ctx)
		if err != nil {
			return safeStoreFailure(err)
		}
		sort.Strings(keys)
		for _, key := range keys {
			// Obsolete payloads need no approval because they are discarded unread.
			owned := key == credentialRevisionKey || key == snapshotFormatKey ||
				strings.HasPrefix(key, credentialPrefix) || strings.HasPrefix(key, workloadPrefix) ||
				strings.HasPrefix(key, objectChunksPrefix)
			if !owned {
				continue
			}
			if _, _, err := s.native.Get(ctx, key); err != nil {
				return safeStoreFailure(err)
			}
		}
		return nil
	})
}

func (s *RecordStore) SetCredential(ctx context.Context, name, provider, value string) error {
	if !validCredential(name, provider, value) {
		return ErrInvalidCredential
	}
	return s.withLock(ctx, func() error {
		existing, found, err := s.readCredential(ctx, name)
		if err != nil {
			return err
		}
		if found && existing.Provider != provider {
			return ErrCredentialProviderMismatch
		}
		record := credentialRecord{SchemaVersion: 1, Kind: "credential", Name: name, Provider: provider, Value: value}
		data, err := json.Marshal(record)
		if err != nil {
			return ErrInvalidCredential
		}
		if _, err := s.initialize(ctx); err != nil {
			return err
		}
		if _, err := s.advanceRevision(ctx); err != nil {
			return err
		}
		return s.writeObject(ctx, credentialPrefix+name, data)
	})
}

func (s *RecordStore) ReadCredential(ctx context.Context, name, provider string) (string, error) {
	if !source.ValidCredentialName(name) || !source.KnownProvider(provider) {
		return "", ErrInvalidCredential
	}
	var value string
	err := s.withLock(ctx, func() error {
		record, found, err := s.readCredential(ctx, name)
		if err != nil {
			return err
		}
		if !found {
			return ErrCredentialNotFound
		}
		if record.Provider != provider {
			return ErrCredentialProviderMismatch
		}
		value = record.Value
		return nil
	})
	if err != nil {
		return "", err
	}
	return value, nil
}

func (s *RecordStore) RemoveCredential(ctx context.Context, name string) (CredentialMetadata, error) {
	if !source.ValidCredentialName(name) {
		return CredentialMetadata{}, ErrInvalidCredential
	}
	var removed CredentialMetadata
	err := s.withLock(ctx, func() error {
		record, found, err := s.readCredential(ctx, name)
		if err != nil {
			return err
		}
		if !found {
			if err := s.deleteObject(ctx, credentialPrefix+name); err != nil {
				return err
			}
			return ErrCredentialNotFound
		}
		if _, err := s.initialize(ctx); err != nil {
			return err
		}
		if _, err := s.advanceRevision(ctx); err != nil {
			return err
		}
		if err := s.deleteObject(ctx, credentialPrefix+name); err != nil {
			return err
		}
		removed = CredentialMetadata{Name: name, Provider: record.Provider}
		return nil
	})
	if err != nil {
		return CredentialMetadata{}, err
	}
	return removed, nil
}

func (s *RecordStore) AcquireCredentials(ctx context.Context, requests []CredentialRequest) (CredentialContext, error) {
	for _, request := range requests {
		if !source.ValidCredentialName(request.Name) || !source.KnownProvider(request.Provider) {
			return CredentialContext{}, ErrInvalidCredential
		}
	}
	credentials := CredentialContext{values: make(map[string]credentialRecord, len(requests))}
	err := s.withLock(ctx, func() error {
		revision, err := s.initialize(ctx)
		if err != nil {
			return err
		}
		credentials.Revision = revision
		for _, request := range requests {
			record, found, err := s.readCredential(ctx, request.Name)
			if err != nil {
				return err
			}
			if !found {
				return ErrCredentialNotFound
			}
			if record.Provider != request.Provider {
				return ErrCredentialProviderMismatch
			}
			credentials.values[request.Name] = record
		}
		return nil
	})
	if err != nil {
		return CredentialContext{}, err
	}
	return credentials, nil
}

func (s *RecordStore) ListCredentials(ctx context.Context) ([]CredentialMetadata, error) {
	metadata := make([]CredentialMetadata, 0)
	err := s.withLock(ctx, func() error {
		keys, err := s.native.Keys(ctx)
		if err != nil {
			return safeStoreFailure(err)
		}
		for _, key := range keys {
			if !strings.HasPrefix(key, credentialPrefix) {
				continue
			}
			name := strings.TrimPrefix(key, credentialPrefix)
			if !source.ValidCredentialName(name) {
				return ErrRecordCorrupt
			}
			record, found, err := s.readCredential(ctx, name)
			if err != nil {
				return err
			}
			if found {
				metadata = append(metadata, CredentialMetadata{Name: name, Provider: record.Provider})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(metadata, func(i, j int) bool { return metadata[i].Name < metadata[j].Name })
	return metadata, nil
}

func (s *RecordStore) ReadSnapshot(ctx context.Context, scope string, ttl time.Duration) (WorkloadSnapshot, bool, error) {
	if !scopePattern.MatchString(scope) {
		return WorkloadSnapshot{}, false, ErrInvalidSnapshot
	}
	var cached WorkloadSnapshot
	var hit bool
	err := s.withLock(ctx, func() error {
		revision, err := s.initialize(ctx)
		if err != nil {
			return err
		}
		data, found, err := s.readObject(ctx, workloadPrefix+scope)
		if err != nil || !found {
			return err
		}
		var record workloadRecord
		if err := decodeRecord(data, &record, "schema_version", "kind", "scope", "credential_revision", "snapshot"); err != nil {
			return err
		}
		if record.SchemaVersion != 2 || record.Kind != "snapshot" || record.Scope != scope ||
			!validRevision(record.Revision) || !validSnapshot(record.Snapshot) {
			return ErrRecordCorrupt
		}
		if record.Revision != revision || record.Snapshot.IsExpired(ttl) {
			return nil
		}
		cached = WorkloadSnapshot{Secrets: record.Snapshot, Revision: revision}
		hit = true
		return nil
	})
	if err != nil {
		return WorkloadSnapshot{}, false, err
	}
	return cached, hit, nil
}

func (s *RecordStore) WriteSnapshot(ctx context.Context, scope, revision string, snapshot *secret.Secrets) error {
	if !scopePattern.MatchString(scope) || !validSnapshot(snapshot) {
		return ErrInvalidSnapshot
	}
	return s.withLock(ctx, func() error {
		if err := s.checkRevision(ctx, revision); err != nil {
			return err
		}
		record := workloadRecord{SchemaVersion: 2, Kind: "snapshot", Scope: scope, Revision: revision, Snapshot: snapshot}
		data, err := json.Marshal(record)
		if err != nil {
			return ErrInvalidSnapshot
		}
		return s.writeObject(ctx, workloadPrefix+scope, data)
	})
}

func (s *RecordStore) CleanSnapshots(ctx context.Context) error {
	return s.withLock(ctx, func() error { return s.cleanSnapshots(ctx) })
}

func (s *RecordStore) CheckRevision(ctx context.Context, revision string) error {
	return s.withLock(ctx, func() error { return s.checkRevision(ctx, revision) })
}

func (s *RecordStore) Close() error {
	<-s.gate
	defer func() { s.gate <- struct{}{} }()
	if !s.closed {
		s.closed = true
		s.closeErr = safeStoreFailure(s.native.Close())
	}
	return s.closeErr
}

func (s *RecordStore) withLock(ctx context.Context, run func() error) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.gate:
	}
	defer func() { s.gate <- struct{}{} }()
	if s.closed {
		return ErrStoreUnavailable
	}
	release, err := s.lock.Lock(ctx)
	if err != nil {
		return safeStoreFailure(err)
	}
	defer func() {
		releaseErr := safeStoreFailure(release())
		err = errors.Join(err, releaseErr, ctx.Err())
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	return run()
}

func (s *RecordStore) readCredential(ctx context.Context, name string) (credentialRecord, bool, error) {
	data, found, err := s.readObject(ctx, credentialPrefix+name)
	if err != nil {
		return credentialRecord{}, false, err
	}
	if !found {
		return credentialRecord{}, false, nil
	}
	var record credentialRecord
	if err := decodeRecord(data, &record, "schema_version", "kind", "name", "provider", "value"); err != nil {
		return credentialRecord{}, false, err
	}
	if record.SchemaVersion != 1 || record.Kind != "credential" ||
		record.Name != name || !validCredential(record.Name, record.Provider, record.Value) {
		return credentialRecord{}, false, ErrRecordCorrupt
	}
	return record, true, nil
}

func (s *RecordStore) initialize(ctx context.Context) (string, error) {
	format, found, err := s.readObject(ctx, snapshotFormatKey)
	if err != nil && !errors.Is(err, ErrRecordCorrupt) {
		return "", err
	}
	if err != nil || !found || !bytes.Equal(format, []byte(snapshotFormat)) {
		if err := s.cleanSnapshots(ctx); err != nil {
			return "", err
		}
		if err := s.writeObject(ctx, snapshotFormatKey, []byte(snapshotFormat)); err != nil {
			return "", err
		}
	}
	data, found, err := s.readObject(ctx, credentialRevisionKey)
	if err != nil && !errors.Is(err, ErrRecordCorrupt) {
		return "", err
	}
	if err == nil && found && validRevision(string(data)) {
		return string(data), nil
	}
	if err := s.cleanSnapshots(ctx); err != nil {
		return "", err
	}
	return s.advanceRevision(ctx)
}

func (s *RecordStore) advanceRevision(ctx context.Context) (string, error) {
	revision, err := randomRecordID()
	if err != nil {
		return "", err
	}
	if err := s.writeObject(ctx, credentialRevisionKey, []byte(revision)); err != nil {
		return "", err
	}
	return revision, nil
}

func (s *RecordStore) checkRevision(ctx context.Context, expected string) error {
	current, err := s.initialize(ctx)
	if err != nil {
		return err
	}
	if expected != current {
		return ErrCredentialContextChanged
	}
	return nil
}

func (s *RecordStore) cleanSnapshots(ctx context.Context) error {
	keys, err := s.native.Keys(ctx)
	if err != nil {
		return safeStoreFailure(err)
	}
	for _, key := range keys {
		workload := strings.HasPrefix(key, workloadPrefix) && scopePattern.MatchString(strings.TrimPrefix(key, workloadPrefix))
		legacy := key == "secret_inject" || strings.HasPrefix(key, cachePrefix) && scopePattern.MatchString(strings.TrimPrefix(key, cachePrefix))
		if workload || legacy {
			if err := s.deleteObject(ctx, key); err != nil {
				return err
			}
		}
	}
	return s.removeRoleChunks(ctx, strings.TrimSuffix(workloadPrefix, "/"))
}

func decodeRecord(data []byte, target any, keys ...string) error {
	fields, err := jsonobject.Decode(data)
	if err != nil || len(fields) != len(keys) {
		return ErrRecordCorrupt
	}
	for _, key := range keys {
		if _, found := fields[key]; !found {
			return ErrRecordCorrupt
		}
	}
	if err := json.Unmarshal(data, target); err != nil {
		return ErrRecordCorrupt
	}
	return nil
}

func validRevision(revision string) bool {
	decoded, err := hex.DecodeString(revision)
	return err == nil && len(decoded) == 16 && revision == strings.ToLower(revision)
}

// JSON must not replace invalid UTF-8 in persisted secret bytes.
func validSnapshot(snapshot *secret.Secrets) bool {
	if snapshot == nil || snapshot.Entries == nil || snapshot.Timestamp.IsZero() {
		return false
	}
	for key, value := range snapshot.Entries {
		if !utf8.ValidString(key) || !utf8.ValidString(value) {
			return false
		}
	}
	return true
}

func validCredential(name, provider, value string) bool {
	return source.ValidCredentialName(name) && source.KnownProvider(provider) && source.ValidToken(value)
}

func safeStoreFailure(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	case errors.Is(err, ErrInteractionRequired):
		return ErrInteractionRequired
	case errors.Is(err, ErrStoreUnavailable):
		return ErrStoreUnavailable
	default:
		return ErrStoreOperation
	}
}
