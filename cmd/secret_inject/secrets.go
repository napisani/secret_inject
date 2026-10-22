package main

import (
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

func loadSnapshot(cfg *config.Config, args Args, selected *profile.Profile, debug *diagnostics.Session) (*secret.Secrets, error) {
	snapshot := secret.New()
	if selected.RemoteSources() {
		store, err := storage.Get(cfg.Storage)
		if err != nil {
			return nil, errors.New("cache backend unavailable")
		}
		debug.Record(diagnostics.Event{Kind: diagnostics.StorageProgress, Operation: "initialize", State: "started"})
		if err := store.Initialize(); err != nil {
			return nil, errors.New("cache initialization failed")
		}
		debug.Record(diagnostics.Event{Kind: diagnostics.StorageProgress, Operation: "initialize", State: "completed"})
		snapshot, err = loadMerged(cfg, args, store, debug)
		if err != nil {
			return nil, err
		}
	}
	if patterns := selected.EnvFiles(); len(patterns) > 0 {
		debug.Record(diagnostics.Event{Kind: diagnostics.ProviderState, Provider: "env-files", State: "started"})
		local, err := envfile.Load(patterns)
		if err != nil {
			return nil, err
		}
		debug.Record(diagnostics.Event{Kind: diagnostics.ProviderState, Provider: "env-files", State: "completed"})
		snapshot = appendSnapshot(snapshot, local, debug)
	}
	return snapshot, nil
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

func loadMerged(
	cfg *config.Config,
	args Args,
	store storage.Storage,
	debug *diagnostics.Session,
) (*secret.Secrets, error) {
	sources, err := source.LoadAll(cfg.Sources, cfg.SourceSequence)
	if err != nil {
		return nil, errors.New("provider initialization failed; check source config and installed CLIs")
	}
	scope, err := storage.SourceScope(cfg.Sources, cfg.SourceSequence)
	if err != nil {
		return nil, errors.New("invalid source order or configuration")
	}
	if !args.Force {
		snapshot, found, err := store.Read(scope)
		if err != nil {
			return nil, errors.New("cached secret snapshot is unreadable or invalid")
		}
		if found && !snapshot.IsExpired(args.TTL) {
			debug.Record(diagnostics.Event{Kind: diagnostics.CacheState, State: "hit"})
			return snapshot, nil
		}
		state := "miss"
		if found {
			state = "expired"
		}
		debug.Record(diagnostics.Event{Kind: diagnostics.CacheState, State: state})
	} else {
		debug.Record(diagnostics.Event{Kind: diagnostics.CacheState, State: "forced"})
	}
	merged := secret.New()
	for _, src := range sources {
		name := source.Name(src)
		debug.Record(diagnostics.Event{Kind: diagnostics.ProviderState, Provider: name, State: "started"})
		more, err := src.GetAllSecrets(merged)
		if err != nil {
			var failure *source.ProviderError
			if errors.As(err, &failure) {
				var exitCode *int
				if failure.Kind == "exit" {
					exitCode = &failure.ExitCode
				}
				debug.Record(diagnostics.Event{
					Kind:      diagnostics.ProviderFailed,
					Provider:  failure.Provider,
					Operation: failure.Operation,
					Failure:   failure.Kind,
					ExitCode:  exitCode,
				})
				return nil, failure
			}
			return nil, errors.New("provider fetch failed")
		}
		debug.Record(diagnostics.Event{Kind: diagnostics.ProviderState, Provider: name, State: "completed"})
		merged = appendSnapshot(merged, more, debug)
	}
	if err := store.Write(scope, merged); err != nil {
		return nil, errors.New("cache snapshot write failed")
	}
	return merged, nil
}
