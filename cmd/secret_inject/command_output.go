package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/napisani/secret_inject/internal/source"
	"github.com/napisani/secret_inject/internal/storage"
)

type commandFailure uint8

const (
	failureUsage commandFailure = iota
	failureConfiguration
	failureProfileSelection
	failureMetadataOutput
	failureUnsupportedPlatform
	failureExecutableMissing
	failureExecutablePermission
	failureInteraction
	failureStoreUnavailable
	failureStoreOperation
	failureCredentialMissing
	failureCredentialProvider
	failureCredentialContext
	failureProviderVersion
	failureProvider
	failureProviderTimeout
	failureProjection
	failureEnvironment
	failureFinalization
	failureInterrupted
	failureTerminated
)

type commandError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorResponse struct {
	SchemaVersion int          `json:"schema_version"`
	Error         commandError `json:"error"`
}

func reportCommandFailure(stderr io.Writer, failure commandFailure) int {
	var detail commandError
	status := 1
	switch failure {
	case failureUsage:
		detail = commandError{"usage", "invalid invocation; use --help for usage"}
		status = 2
	case failureConfiguration:
		detail = commandError{"configuration", "configuration could not be read or validated"}
	case failureProfileSelection:
		detail = commandError{"profile_selection", "profile is unavailable"}
	case failureMetadataOutput:
		detail = commandError{"metadata_output", "metadata output failed"}
	case failureUnsupportedPlatform:
		detail = commandError{"unsupported_platform", "command is unsupported on this platform"}
	case failureExecutableMissing:
		detail = commandError{"executable_not_found", "target executable is unavailable"}
		status = 127
	case failureExecutablePermission:
		detail = commandError{"executable_not_executable", "target cannot be executed"}
		status = 126
	case failureInteraction:
		detail = commandError{"interaction_required", "native store requires explicit preparation"}
	case failureStoreUnavailable:
		detail = commandError{"store_unavailable", "native store is unavailable"}
	case failureStoreOperation:
		detail = commandError{"store_operation", "native store operation failed"}
	case failureCredentialMissing:
		detail = commandError{"credential_missing", "credential is unavailable"}
	case failureCredentialProvider:
		detail = commandError{"credential_provider_mismatch", "credential provider does not match the source"}
	case failureCredentialContext:
		detail = commandError{"credential_context_changed", "credential context changed; restart resolution"}
	case failureProviderVersion:
		detail = commandError{"provider_version_unsupported", "provider version is unsupported"}
	case failureProvider:
		detail = commandError{"provider_failure", "provider operation failed"}
	case failureProviderTimeout:
		detail = commandError{"provider_timeout", "provider operation timed out"}
	case failureProjection:
		detail = commandError{"projection", "profile projection failed"}
	case failureEnvironment:
		detail = commandError{"invalid_environment", "target environment is invalid"}
	case failureFinalization:
		detail = commandError{"resource_finalization", "preparation finalization failed"}
	case failureInterrupted:
		detail = commandError{"cancelled", "preparation interrupted"}
		status = 130
	case failureTerminated:
		detail = commandError{"cancelled", "preparation terminated"}
		status = 143
	default:
		detail = commandError{"configuration", "operation failed"}
	}
	if err := writeJSON(stderr, errorResponse{SchemaVersion: 1, Error: detail}); err != nil {
		return 1
	}
	return status
}

func preparationFailure(ctx context.Context, err error) commandFailure {
	if errors.Is(context.Cause(ctx), errTerminated) {
		return failureTerminated
	}
	if errors.Is(context.Cause(ctx), errInterrupted) || errors.Is(err, context.Canceled) {
		return failureInterrupted
	}
	switch {
	case errors.Is(err, errResourceFinalization), errors.Is(err, source.ErrProviderCleanup):
		return failureFinalization
	case errors.Is(err, storage.ErrInteractionRequired):
		return failureInteraction
	case errors.Is(err, storage.ErrStoreUnavailable):
		return failureStoreUnavailable
	case errors.Is(err, storage.ErrCredentialNotFound), errors.Is(err, source.ErrPrivateCredential):
		return failureCredentialMissing
	case errors.Is(err, storage.ErrCredentialProviderMismatch):
		return failureCredentialProvider
	case errors.Is(err, storage.ErrCredentialContextChanged):
		return failureCredentialContext
	case errors.Is(err, source.ErrProviderVersion):
		return failureProviderVersion
	case errors.Is(err, storage.ErrRecordCorrupt), errors.Is(err, storage.ErrStoreOperation):
		return failureStoreOperation
	case errors.Is(err, source.ErrCredentialBinding):
		return failureConfiguration
	}
	var provider *source.ProviderError
	if errors.As(err, &provider) {
		if provider.Kind == "timeout" {
			return failureProviderTimeout
		}
		return failureProvider
	}
	return failureConfiguration
}

func writeJSON(writer io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeOutput(writer, append(data, '\n'))
}
