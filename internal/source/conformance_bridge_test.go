//go:build providerconformance

package source

import (
	"context"
	"encoding/json"
)

func EnableConformanceCandidates(versions map[string]string) func() {
	previous := certifiedProviderReleases
	certifiedProviderReleases = make(map[string]map[string]bool, len(versions))
	for provider, version := range versions {
		certifiedProviderReleases[provider] = map[string]bool{version: true}
	}
	return func() { certifiedProviderReleases = previous }
}

func FetchConformanceCandidate(ctx context.Context, provider string, raw json.RawMessage, token string) (map[string]string, error) {
	return fetchPrivateProvider(ctx, provider, raw, token)
}
