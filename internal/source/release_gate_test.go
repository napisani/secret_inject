//go:build releasegate

package source

import (
	"strings"
	"testing"
)

// Entries require reviewed native/provider conformance evidence.
var certifiedProviderReleases = map[string]map[string]bool{}

func certifiedProviderReady(provider string) bool {
	for version, verified := range certifiedProviderReleases[provider] {
		if verified && version != "" && version == strings.TrimSpace(version) && !strings.ContainsAny(version, "\r\n\x00") {
			return true
		}
	}
	return false
}

func TestReleaseRequiresCertifiedUnattendedProviders(t *testing.T) {
	for provider := range sourceRegistry {
		if !certifiedProviderReady(provider) {
			t.Errorf("provider %s has no certified unattended release; release is blocked", provider)
		}
	}
}
