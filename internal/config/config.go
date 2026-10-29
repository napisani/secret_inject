package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/napisani/secret_inject/internal/profile"
)

// Config is the top-level config file. Each source decodes and validates its
// own entry in Sources.
type Config struct {
	Sources        map[string]json.RawMessage `json:"sources"`
	Storage        *StorageConfig             `json:"storage"`
	SourceSequence []string                   `json:"source_sequence"`
	Profiles       profile.Profiles           `json:"profiles"`
}

// StorageConfig selects the shared credential and workload store.
// AllowedBackends selects at most one backend per OS; only the host's is used.
type StorageConfig struct {
	Type            string   `json:"type"`
	AllowedBackends []string `json:"allowed_backends"`
	Password        string   `json:"password"`
}

func (c *Config) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return errors.New("configuration must be an object")
	}
	profilesSeen := false
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if name, ok := token.(string); ok && strings.EqualFold(name, "profiles") {
			if name != "profiles" {
				return errors.New("profiles field must use lowercase spelling")
			}
			if profilesSeen {
				return errors.New("duplicate profiles block")
			}
			profilesSeen = true
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
	}
	type wire Config
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*c = Config(parsed)
	return nil
}

func ReadConfig(filename string) (*Config, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	err = json.Unmarshal(content, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
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
