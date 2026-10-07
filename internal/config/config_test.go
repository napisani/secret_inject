package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadConfig(t *testing.T) {
	// Create a temporary config file
	content := `{
		"sources": {
			"doppler": {
				"project": "test-project",
				"env": "dev"
			}
		},
		"storage": {
			"type": "file"
		}
	}`

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")

	err := os.WriteFile(configFile, []byte(content), 0600)
	if err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Test reading config
	cfg, err := ReadConfig(configFile)
	if err != nil {
		t.Fatalf("ReadConfig failed: %v", err)
	}

	if cfg.Sources == nil {
		t.Error("Sources is nil")
	}

	if cfg.Storage == nil {
		t.Error("Storage is nil")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Sources: map[string]json.RawMessage{
					"doppler": json.RawMessage(`{"project": "test", "env": "dev"}`),
				},
				Storage: &StorageConfig{Type: "file"},
			},
			wantErr: false,
		},
		{
			name: "missing storage",
			config: &Config{
				Sources: map[string]json.RawMessage{},
			},
			wantErr: true,
		},
		{
			name: "missing storage type",
			config: &Config{
				Sources: map[string]json.RawMessage{},
				Storage: &StorageConfig{},
			},
			wantErr: true,
		},
		{
			name: "invalid storage type",
			config: &Config{
				Sources: map[string]json.RawMessage{},
				Storage: &StorageConfig{Type: "invalid"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestReadConfigInvalidFile(t *testing.T) {
	_, err := ReadConfig("/nonexistent/file.json")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestReadConfigInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")

	err := os.WriteFile(configFile, []byte("invalid json"), 0600)
	if err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	_, err = ReadConfig(configFile)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestReadConfigDecodesStorage(t *testing.T) {
	content := `{
		"storage": {
			"type": "keyring",
			"allowed_backends": ["keychain"],
			"password": "pw"
		}
	}`
	configFile := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	cfg, err := ReadConfig(configFile)
	if err != nil {
		t.Fatalf("ReadConfig failed: %v", err)
	}
	got := cfg.Storage
	if got.Type != "keyring" || len(got.AllowedBackends) != 1 || got.AllowedBackends[0] != "keychain" || got.Password != "pw" {
		t.Fatalf("unexpected storage config: type=%q backends=%v", got.Type, got.AllowedBackends)
	}
}

func TestReadConfigRejectsMistypedStorage(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configFile, []byte(`{"storage": {"type": 5}}`), 0600); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	if _, err := ReadConfig(configFile); err == nil {
		t.Fatal("expected an error for a non-string storage type")
	}
}
