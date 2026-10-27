package main

import (
	"context"
	"errors"
	"sort"

	"github.com/napisani/secret_inject/internal/config"
	"github.com/napisani/secret_inject/internal/diagnostics"
	"github.com/napisani/secret_inject/internal/envfile"
	"github.com/napisani/secret_inject/internal/profile"
	"github.com/napisani/secret_inject/internal/secret"
	"github.com/napisani/secret_inject/internal/source"
	"github.com/napisani/secret_inject/internal/storage"
)

var errResourceFinalization = errors.New("preparation finalization failed")
var openProtectedStore = storage.OpenNative
var fetchRemoteProvider source.ProviderFetch

type resolvedSnapshot struct {
	Values   *secret.Secrets
	store    *storage.RecordStore
	revision string
}

func (r *resolvedSnapshot) Close() error {
	if r.store == nil {
		return nil
	}
	err := r.store.Close()
	r.store = nil
	if err != nil {
		return errors.Join(errResourceFinalization, err)
	}
	return nil
}
func (r *resolvedSnapshot) Finish(ctx context.Context) error {
	var err error
	if r.store != nil {
		err = r.store.CheckRevision(ctx, r.revision)
	}
	return errors.Join(err, r.Close(), ctx.Err())
}

func loadSnapshot(ctx context.Context, cfg *config.Config, args Args, selected *profile.Profile, debug *diagnostics.Session) (_ *resolvedSnapshot, err error) {
	result := &resolvedSnapshot{Values: secret.New()}
	defer func() {
		if err != nil {
			err = errors.Join(err, result.Close())
		}
	}()
	if selected.RemoteSources() {
		plan, err := source.PlanCredentials(cfg.Sources, cfg.SourceSequence)
		if err != nil {
			return nil, err
		}
		if err := storage.ValidateNativeConfig(cfg.Storage); err != nil {
			return nil, err
		}
		scope, err := storage.SourceScope(cfg.Sources, cfg.SourceSequence)
		if err != nil {
			return nil, errors.New("invalid source configuration")
		}
		result.store, err = openProtectedStore(ctx, cfg.Storage, storage.Unattended)
		if err != nil {
			return nil, err
		}
		var hit bool
		if !args.Force {
			cached, found, err := result.store.ReadSnapshot(ctx, scope, args.TTL)
			if err != nil {
				return nil, err
			}
			if found {
				result.Values = cached.Secrets
				result.revision = cached.Revision
				hit = true
			}
		}
		if hit {
			debug.Record(diagnostics.Event{Kind: diagnostics.CacheState, State: "hit"})
		} else {
			debug.Record(diagnostics.Event{Kind: diagnostics.CacheState, State: "miss"})
			requests := make([]storage.CredentialRequest, 0, len(plan))
			for _, entry := range plan {
				if binding, ok := entry.Binding.(source.StoredBinding); ok {
					requests = append(requests, storage.CredentialRequest{Name: binding.Name, Provider: entry.Provider})
				}
			}
			credentials, err := result.store.AcquireCredentials(ctx, requests)
			if err != nil {
				return nil, err
			}
			debug.Record(diagnostics.Event{Kind: diagnostics.ProviderState, Operation: "resolve", State: "started"})
			result.Values, err = source.ResolvePrivate(ctx, cfg.Sources, cfg.SourceSequence, credentials.Value, fetchRemoteProvider)
			if err != nil {
				var failure *source.ProviderError
				if errors.As(err, &failure) {
					debug.Record(diagnostics.Event{Kind: diagnostics.ProviderFailed, Provider: failure.Provider, Operation: failure.Operation, Failure: failure.Kind})
				}
				return nil, err
			}
			result.revision = credentials.Revision
			if err := result.store.WriteSnapshot(ctx, scope, result.revision, result.Values); err != nil {
				return nil, err
			}
			debug.Record(diagnostics.Event{Kind: diagnostics.ProviderState, Operation: "resolve", State: "completed"})
		}
	}
	if patterns := selected.EnvFiles(); len(patterns) > 0 {
		debug.Record(diagnostics.Event{Kind: diagnostics.ProviderState, Provider: "env-files", State: "started"})
		local, err := envfile.Load(patterns)
		if err != nil {
			return nil, err
		}
		result.Values = appendSnapshot(result.Values, local, debug)
		debug.Record(diagnostics.Event{Kind: diagnostics.ProviderState, Provider: "env-files", State: "completed"})
	}
	return result, nil
}

func appendSnapshot(previous, more *secret.Secrets, debug *diagnostics.Session) *secret.Secrets {
	collisions := []string{}
	for key := range more.Entries {
		if _, exists := previous.Entries[key]; exists {
			collisions = append(collisions, key)
		}
	}
	sort.Strings(collisions)
	for _, key := range collisions {
		debug.Record(diagnostics.Event{Kind: diagnostics.SourceOverwrite, Original: key})
	}
	return previous.Append(more)
}
