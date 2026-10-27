//go:build providerconformance

package source_test

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"testing"

	"github.com/napisani/secret_inject/internal/config"
	"github.com/napisani/secret_inject/internal/conformance"
	"github.com/napisani/secret_inject/internal/source"
	"github.com/napisani/secret_inject/internal/storage"
)

func TestProviderConformanceDedicatedAccounts(t *testing.T) {
	if err := conformance.RequireDisposableAccount(); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.ReadConfig(os.Getenv("SECRET_INJECT_CONFORMANCE_CONFIG"))
	if err != nil {
		t.Fatal("dedicated provider fixture configuration is unavailable")
	}
	var versions map[string]string
	if err := json.Unmarshal([]byte(os.Getenv("SECRET_INJECT_CONFORMANCE_VERSIONS")), &versions); err != nil {
		t.Fatal("exact candidate version data is required")
	}
	versionSyntax := regexp.MustCompile(`^(bws )?v?[0-9]+\.[0-9]+\.[0-9]+([-.][A-Za-z0-9.]+)?$`)
	for _, provider := range []string{"doppler", "onepassword", "bitwarden"} {
		if !versionSyntax.MatchString(versions[provider]) {
			t.Fatal("a candidate version is missing or malformed")
		}
	}
	restore := source.EnableConformanceCandidates(versions)
	defer restore()
	plan, err := source.PlanCredentials(cfg.Sources, cfg.SourceSequence)
	if err != nil || len(plan) != 3 {
		t.Fatal("provider fixture needs exactly three bound sources")
	}
	requests := make([]storage.CredentialRequest, 0, 3)
	for _, entry := range plan {
		binding, ok := entry.Binding.(source.StoredBinding)
		if !ok {
			t.Fatal("direct provider fixture needs one dedicated stored root per provider")
		}
		requests = append(requests, storage.CredentialRequest{Name: binding.Name, Provider: entry.Provider})
	}
	store, err := storage.OpenNative(t.Context(), cfg.Storage, storage.Unattended)
	if err != nil {
		t.Fatal("dedicated provider roots are unavailable")
	}
	credentials, readErr := store.AcquireCredentials(t.Context(), requests)
	closeErr := store.Close()
	if readErr != nil || closeErr != nil {
		t.Fatal("dedicated provider root acquisition/finalization failed")
	}
	for _, request := range requests {
		t.Run(request.Provider, func(t *testing.T) {
			token, err := credentials.Value(request.Name, request.Provider)
			if err != nil {
				t.Fatal("dedicated root unavailable")
			}
			values, err := source.FetchConformanceCandidate(t.Context(), request.Provider, cfg.Sources[request.Provider], token)
			if err != nil || values["CONFORMANCE_OK"] != "synthetic-fixture-ok" {
				t.Fatal("candidate did not resolve the dedicated synthetic fixture")
			}
			values, err = source.FetchConformanceCandidate(t.Context(), request.Provider, cfg.Sources[request.Provider], "invalid-synthetic-conformance-token")
			var providerFailure *source.ProviderError
			if err == nil || values != nil || !errors.As(err, &providerFailure) {
				t.Fatal("invalid candidate auth did not fail through the controlled provider path")
			}
		})
	}
}
