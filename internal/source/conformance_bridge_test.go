//go:build providerconformance

package source

import (
	"context"
	"encoding/json"
	"errors"
	"os"
)

func ProbeConformanceVersion(ctx context.Context, provider string) (version string, err error) {
	private, err := newProviderContext(provider)
	if err != nil {
		return "", err
	}
	defer func() {
		if cleanupErr := os.RemoveAll(private.directory); cleanupErr != nil {
			version = ""
			err = errors.Join(err, ErrProviderCleanup)
		}
	}()
	return private.version(ctx)
}

func FetchConformanceCandidate(ctx context.Context, provider string, raw json.RawMessage, token string) (map[string]string, error) {
	return fetchPrivateProvider(ctx, provider, raw, token)
}
