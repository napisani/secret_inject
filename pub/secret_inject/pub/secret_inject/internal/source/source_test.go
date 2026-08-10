package source

import (
	"errors"
	"fmt"
	"testing"

	"github.com/napisani/secret_inject/internal/secret"
)

func withPatchedGlobals(run func(string, []string, ...string) ([]byte, error), look func(string) (string, error)) func() {
	originalRun := runCLICommand
	originalLookup := lookupBinary
	if run != nil {
		runCLICommand = run
	}
	if look != nil {
		lookupBinary = look
	}
	return func() {
		runCLICommand = originalRun
		lookupBinary = originalLookup
	}
}

func hasEnvVar(env []string, key string, value string) bool {
	target := fmt.Sprintf("%s=%s", key, value)
	for _, entry := range env {
		if entry == target {
			return true
		}
	}
	return false
}

func TestLoadAllReturnsErrorForUnknownSource(t *testing.T) {
	cfg := map[string]interface{}{
		"sources": map[string]interface{}{
			"unknown": map[string]interface{}{},
		},
	}

	if _, err := LoadAll(cfg); err == nil {
		t.Fatalf("expected error for unknown source")
	}
}

func TestLoadAllRespectsSequence(t *testing.T) {
	cfg := map[string]interface{}{
		"source_sequence": []interface{}{"onepassword", "doppler"},
		"sources": map[string]interface{}{
			"doppler": map[string]interface{}{
				"project": "proj",
				"env":     "dev",
			},
			"onepassword": map[string]interface{}{
				"secrets": map[string]interface{}{
					"API_KEY": "op://vault/item/password",
				},
			},
			"bitwarden": map[string]interface{}{
				"secrets": map[string]interface{}{
					"DB_PASSWORD": "123",
				},
			},
		},
	}

	cleanup := withPatchedGlobals(nil, func(string) (string, error) {
		return "/usr/bin/mock", nil
	})
	defer cleanup()

	sources, err := LoadAll(cfg)
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if len(sources) != 3 {
		t.Fatalf("expected 3 sources, got %d", len(sources))
	}

	if _, ok := sources[0].(*OnePassword); !ok {
		t.Fatalf("expected OnePassword first, got %T", sources[0])
	}
	if _, ok := sources[1].(*Doppler); !ok {
		t.Fatalf("expected Doppler second, got %T", sources[1])
	}
	if _, ok := sources[2].(*Bitwarden); !ok {
		t.Fatalf("expected Bitwarden third, got %T", sources[2])
	}
}

func TestOnePasswordSourceFetchesSecrets(t *testing.T) {
	cfg := map[string]interface{}{
		"sources": map[string]interface{}{
			"onepassword": map[string]interface{}{
				"secrets": map[string]interface{}{
					"API_KEY":  "op://vault/item/password",
					"USERNAME": "op://vault/item/username",
				},
			},
		},
	}

	cleanup := withPatchedGlobals(func(name string, env []string, args ...string) ([]byte, error) {
		if name != "op" {
			return nil, fmt.Errorf("unexpected binary %s", name)
		}
		if !hasEnvVar(env, "OP_SERVICE_ACCOUNT_TOKEN", "token") {
			return nil, fmt.Errorf("missing expected env var")
		}
		if len(args) != 2 || args[0] != "read" {
			return nil, fmt.Errorf("unexpected args %v", args)
		}

		switch args[1] {
		case "op://vault/item/password":
			return []byte("secret-value\n"), nil
		case "op://vault/item/username":
			return []byte("user\n"), nil
		default:
			return nil, fmt.Errorf("unexpected reference %s", args[1])
		}
	}, func(string) (string, error) {
		return "/usr/bin/op", nil
	})
	defer cleanup()

	sources, err := LoadAll(cfg)
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}

	opSource, ok := sources[0].(*OnePassword)
	if !ok {
		t.Fatalf("expected OnePassword source, got %T", sources[0])
	}

	previous := secret.New()
	previous.Entries["OP_SERVICE_ACCOUNT_TOKEN"] = "token"

	secrets, err := opSource.GetAllSecrets(previous)
	if err != nil {
		t.Fatalf("GetAllSecrets failed: %v", err)
	}

	want := map[string]string{
		"API_KEY":  "secret-value",
		"USERNAME": "user",
	}

	if len(secrets.Entries) != len(want) {
		t.Fatalf("unexpected secrets count: got %d want %d", len(secrets.Entries), len(want))
	}

	for key, value := range want {
		if got := secrets.Entries[key]; got != value {
			t.Errorf("secret %s: got %q want %q", key, got, value)
		}
	}
}

func TestOnePasswordInitMissingCLI(t *testing.T) {
	cfg := map[string]interface{}{
		"sources": map[string]interface{}{
			"onepassword": map[string]interface{}{
				"secrets": map[string]interface{}{
					"API_KEY": "op://vault/item/password",
				},
			},
		},
	}

	cleanup := withPatchedGlobals(nil, func(string) (string, error) {
		return "", errors.New("not found")
	})
	defer cleanup()

	if _, err := LoadAll(cfg); err == nil {
		t.Fatalf("expected error when op CLI missing")
	}
}

func TestBitwardenSourceFetchesByIDAndKey(t *testing.T) {
	cfg := map[string]interface{}{
		"sources": map[string]interface{}{
			"bitwarden": map[string]interface{}{
				"secrets": map[string]interface{}{
					"DB_PASSWORD": "123",
					"API_TOKEN": map[string]interface{}{
						"key":        "api-token",
						"project_id": "proj-1",
					},
				},
			},
		},
	}

	cleanup := withPatchedGlobals(func(name string, env []string, args ...string) ([]byte, error) {
		if name != "bws" {
			return nil, fmt.Errorf("unexpected binary %s", name)
		}
		if !hasEnvVar(env, "BWS_ACCESS_TOKEN", "token") {
			return nil, fmt.Errorf("missing expected env var")
		}

		if len(args) >= 2 && args[0] == "secret" && args[1] == "get" {
			return []byte(`{"id":"123","key":"DB_PASSWORD","value":"pass"}`), nil
		}

		if len(args) >= 2 && args[0] == "secret" && args[1] == "list" {
			return []byte(`[{"id":"abc","key":"api-token","value":"token"}]`), nil
		}

		return nil, fmt.Errorf("unexpected args %v", args)
	}, func(string) (string, error) {
		return "/usr/bin/bws", nil
	})
	defer cleanup()

	sources, err := LoadAll(cfg)
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}

	bwSource, ok := sources[0].(*Bitwarden)
	if !ok {
		t.Fatalf("expected Bitwarden source, got %T", sources[0])
	}

	previous := secret.New()
	previous.Entries["BWS_ACCESS_TOKEN"] = "token"

	secrets, err := bwSource.GetAllSecrets(previous)
	if err != nil {
		t.Fatalf("GetAllSecrets failed: %v", err)
	}

	expected := map[string]string{
		"DB_PASSWORD": "pass",
		"API_TOKEN":   "token",
	}

	if len(secrets.Entries) != len(expected) {
		t.Fatalf("unexpected secrets count: got %d want %d", len(secrets.Entries), len(expected))
	}

	for key, value := range expected {
		if got := secrets.Entries[key]; got != value {
			t.Errorf("secret %s: got %q want %q", key, got, value)
		}
	}
}

func TestBitwardenInitValidations(t *testing.T) {
	cleanup := withPatchedGlobals(nil, func(string) (string, error) { return "/usr/bin/bws", nil })
	defer cleanup()

	cases := []map[string]interface{}{
		{
			"sources": map[string]interface{}{
				"bitwarden": map[string]interface{}{
					"secrets": map[string]interface{}{},
				},
			},
		},
		{
			"sources": map[string]interface{}{
				"bitwarden": map[string]interface{}{
					"secrets": map[string]interface{}{
						"MISSING": map[string]interface{}{},
					},
				},
			},
		},
	}

	for _, cfg := range cases {
		if _, err := LoadAll(cfg); err == nil {
			t.Fatalf("expected validation error for cfg %v", cfg)
		}
	}
}

func TestSourcesDisableWhenLookupFails(t *testing.T) {
	cfg := map[string]interface{}{
		"sources": map[string]interface{}{},
	}

	sources, err := LoadAll(cfg)
	if err != nil {
		t.Fatalf("LoadAll unexpected error: %v", err)
	}

	if len(sources) != 0 {
		t.Fatalf("expected no sources, got %d", len(sources))
	}

	// Directly instantiate to ensure IsEnabled reflects lookup failure
	cleanup := withPatchedGlobals(nil, func(string) (string, error) { return "", errors.New("missing") })
	defer cleanup()

	bw := NewBitwarden()
	if err := bw.Init(map[string]interface{}{"sources": map[string]interface{}{}}); err != nil {
		t.Fatalf("Init should ignore missing config: %v", err)
	}

	if bw.IsEnabled() {
		t.Fatalf("expected bitwarden to be disabled when no config")
	}

	op := NewOnePassword()
	if err := op.Init(map[string]interface{}{"sources": map[string]interface{}{}}); err != nil {
		t.Fatalf("Init should ignore missing config: %v", err)
	}

	if op.IsEnabled() {
		t.Fatalf("expected onepassword to be disabled when no config")
	}
}
