package config

import (
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
				Sources: map[string]interface{}{
					"doppler": map[string]interface{}{
						"project": "test",
						"env":     "dev",
					},
				},
				Storage: map[string]interface{}{
					"type": "file",
				},
			},
			wantErr: false,
		},
		{
			name: "missing storage",
			config: &Config{
				Sources: map[string]interface{}{},
			},
			wantErr: true,
		},
		{
			name: "missing storage type",
			config: &Config{
				Sources: map[string]interface{}{},
				Storage: map[string]interface{}{},
			},
			wantErr: true,
		},
		{
			name: "invalid storage type",
			config: &Config{
				Sources: map[string]interface{}{},
				Storage: map[string]interface{}{
					"type": "invalid",
				},
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
