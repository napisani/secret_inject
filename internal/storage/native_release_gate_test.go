//go:build releasegate

package storage

import "testing"

// Values must identify reviewed, synthetic/private-fixture runtime evidence.
var nativeConformanceEvidence = map[string]string{}

func TestReleaseRequiresNativeRuntimeConformance(t *testing.T) {
	for _, platform := range []string{
		"darwin-keychain/darwin/amd64", "darwin-keychain/darwin/arm64",
		"linux-secret-service/linux/amd64", "linux-secret-service/linux/arm64",
		"linux-systemd-credentials/linux/amd64", "linux-systemd-credentials/linux/arm64",
	} {
		if nativeConformanceEvidence[platform] == "" {
			t.Errorf("native runtime conformance is unverified for %s; release is blocked", platform)
		}
	}
}
