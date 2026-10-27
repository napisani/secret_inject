package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/napisani/secret_inject/internal/config"
	"github.com/napisani/secret_inject/internal/storage"
)

type fixtureEnrollment struct {
	available bool
	value     string
	reads     int
}

func (f *fixtureEnrollment) Available() bool            { return f.available }
func (f *fixtureEnrollment) ReadToken() (string, error) { f.reads++; return f.value, nil }
func installEnrollment(t *testing.T, available bool, value string) *fixtureEnrollment {
	t.Helper()
	previous := enrollmentTerminal
	fixture := &fixtureEnrollment{available: available, value: value}
	enrollmentTerminal = fixture
	t.Cleanup(func() { enrollmentTerminal = previous })
	return fixture
}

func TestCredentialCommandsReturnOnlyMetadataAndUpsertByDefault(t *testing.T) {
	path, _ := commandFixture(t, true)
	input := installEnrollment(t, true, "NEW_PRIVATE_TOKEN_SENTINEL")
	for range 2 {
		code, out, stderr := invoke("credential", "set", "work-root", "--provider", "doppler", "--config", path)
		if code != 0 || stderr != "" || strings.Contains(out, "PRIVATE") {
			t.Fatalf("unsafe credential receipt code=%d stderr=%q", code, stderr)
		}
		assertJSONResponse(t, out, `{"schema_version":1,"credential":{"name":"work-root","provider":"doppler"},"operation":"set"}`)
	}
	if input.reads != 2 {
		t.Fatal("default upsert required an unexpected replacement flag")
	}
	code, out, stderr := invoke("credential", "list", "--config", path)
	if code != 0 || stderr != "" {
		t.Fatalf("list code=%d stderr=%q", code, stderr)
	}
	assertJSONResponse(t, out, `{"schema_version":1,"credentials":[{"name":"work-root","provider":"doppler"}]}`)
	code, out, stderr = invoke("credential", "remove", "work-root", "--config", path)
	if code != 0 || stderr != "" {
		t.Fatalf("remove code=%d stderr=%q", code, stderr)
	}
	assertJSONResponse(t, out, `{"schema_version":1,"credential":{"name":"work-root","provider":"doppler"},"operation":"remove"}`)
	if code, out, stderr := invoke("credential", "remove", "work-root", "--config", path); code != 1 || out != "" || !strings.Contains(stderr, `"credential_missing"`) {
		t.Fatal("missing deletion claimed success")
	}
}

func TestCredentialEnrollmentChecksTerminalAndProviderBeforeTokenInput(t *testing.T) {
	path, _ := commandFixture(t, true)
	input := installEnrollment(t, false, "PRIVATE_TOKEN")
	if code, out, _ := invoke("credential", "set", "work-root", "--provider", "doppler", "--config", path); code != 2 || out != "" || input.reads != 0 {
		t.Fatal("non-terminal enrollment accepted")
	}
	input.available = true
	if code, out, stderr := invoke("credential", "set", "work-root", "--provider", "onepassword", "--config", path); code != 1 || out != "" || input.reads != 0 || !strings.Contains(stderr, "credential_provider_mismatch") {
		t.Fatal("provider mismatch prompted or replaced a credential")
	}
	if code, out, _ := invoke("credential", "set", "../bad", "--provider", "doppler", "--config", path); code != 2 || out != "" || input.reads != 0 {
		t.Fatal("invalid enrollment prompted")
	}
}

func TestStorePreparationDoesNotReplaceCredentialsOrInvalidateSnapshots(t *testing.T) {
	path, _ := commandFixture(t, true)
	cfg, err := config.ReadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	store, err := openProtectedStore(t.Context(), cfg.Storage, storage.Unattended)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := store.AcquireCredentials(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := invoke("store", "prepare", "--config", path)
	if code != 0 || stderr != "" {
		t.Fatalf("prepare code=%d stderr=%q", code, stderr)
	}
	var receipt map[string]any
	if err := json.Unmarshal([]byte(out), &receipt); err != nil {
		t.Fatal(err)
	}
	assertJSONResponse(t, out, `{"schema_version":1,"store":{"kind":"prepared"}}`)
	store, err = openProtectedStore(t.Context(), cfg.Storage, storage.Unattended)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CheckRevision(t.Context(), credentials.Revision); err != nil {
		t.Fatal("prepare invalidated credential generation")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}
