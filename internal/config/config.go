package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
)

type Config struct {
	Sources map[string]interface{} `json:"sources"`
	Storage map[string]interface{} `json:"storage"`
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

	slog.Debug("Read config contents", "sources", len(config.Sources), "storage", len(config.Storage))
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

	storageType, ok := c.Storage["type"].(string)
	if !ok {
		return errors.New("storage 'type' field is required and must be a string")
	}

	if storageType != "keyring" && storageType != "file" {
		return errors.New("storage type must be 'keyring' or 'file'")
	}

	return nil
}
