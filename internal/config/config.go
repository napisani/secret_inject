package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
)

// Config is the top-level config file. Each source decodes and validates its
// own entry in Sources.
type Config struct {
	Sources        map[string]json.RawMessage `json:"sources"`
	Storage        *StorageConfig             `json:"storage"`
	SourceSequence []string                   `json:"source_sequence"`
}

// StorageConfig selects where fetched secrets are cached. AllowedBackends and
// Password apply only to the keyring type.
type StorageConfig struct {
	Type            string   `json:"type"`
	AllowedBackends []string `json:"allowed_backends"`
	Password        string   `json:"password"`
}

func ReadConfig(filename string) (*Config, error) {
	slog.Debug("Reading config file", "filename", filename)

	// Check file permissions first
	checkConfigPermissions(filename)

	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	err = json.Unmarshal(content, &config)
	if err != nil {
		return nil, err
	}

	slog.Debug("Read config contents", "sources", len(config.Sources), "has_storage", config.Storage != nil)
	return &config, nil
}

// checkConfigPermissions warns if config file has insecure permissions (debug mode only)
func checkConfigPermissions(filename string) {
	fileInfo, err := os.Stat(filename)
	if err != nil {
		// File doesn't exist or can't be read, will be handled by ReadFile
		return
	}

	mode := fileInfo.Mode()
	perm := mode.Perm()

	// Check if group or others have any permissions (read, write, or execute)
	if perm&0077 != 0 {
		slog.Debug("Config file has insecure permissions",
			"file", filename,
			"current", fmt.Sprintf("%04o", perm),
			"recommended", "0600",
			"note", "Config may contain sensitive data - consider running: chmod 600 "+filename)
	}
}

func (c *Config) Validate() error {
	if c.Storage == nil {
		return errors.New("storage configuration is required")
	}

	if c.Storage.Type == "" {
		return errors.New("storage 'type' field is required and must be a string")
	}

	if c.Storage.Type != "keyring" && c.Storage.Type != "file" {
		return errors.New("storage type must be 'keyring' or 'file'")
	}

	return nil
}
