//go:build releasegate

package source

import "testing"

func TestReleaseRequiresCertifiedUnattendedProviders(t *testing.T) {
	for provider := range sourceRegistry {
		if !certifiedProviderReady(provider) {
			t.Errorf("provider %s has no certified unattended release; release is blocked", provider)
		}
	}
}
