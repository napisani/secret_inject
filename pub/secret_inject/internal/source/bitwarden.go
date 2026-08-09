package source

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/napisani/secret_inject/internal/secret"
)

type bitwardenSecretConfig struct {
	envVar    string
	id        string
	key       string
	projectID string
}

type Bitwarden struct {
	byID    []bitwardenSecretConfig
	byKey   map[string][]bitwardenSecretConfig
	enabled bool
}

type bitwardenSecret struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

func init() {
	registerSource("bitwarden", func() Source { return NewBitwarden() })
}

func NewBitwarden() *Bitwarden {
	return &Bitwarden{
		byKey: make(map[string][]bitwardenSecretConfig),
	}
}

func (s *Bitwarden) Init(fullConfig map[string]interface{}) error {
	sources, ok := fullConfig["sources"].(map[string]interface{})
	if !ok {
		s.enabled = false
		return nil
	}

	rawConfig, ok := sources["bitwarden"].(map[string]interface{})
	if !ok {
		s.enabled = false
		return nil
	}

	rawSecrets, ok := rawConfig["secrets"].(map[string]interface{})
	if !ok || len(rawSecrets) == 0 {
		s.enabled = false
		return fmt.Errorf("bitwarden source requires a non-empty 'secrets' map")
	}

	var idEntries []bitwardenSecretConfig
	keyEntries := make(map[string][]bitwardenSecretConfig)

	for envVar, value := range rawSecrets {
		switch v := value.(type) {
		case string:
			trimmed := strings.TrimSpace(v)
			if trimmed == "" {
				s.enabled = false
				return fmt.Errorf("bitwarden secret id for %s cannot be empty", envVar)
			}
			idEntries = append(idEntries, bitwardenSecretConfig{envVar: envVar, id: trimmed})
		case map[string]interface{}:
			if idVal, ok := v["id"].(string); ok && strings.TrimSpace(idVal) != "" {
				idEntries = append(idEntries, bitwardenSecretConfig{envVar: envVar, id: strings.TrimSpace(idVal)})
				continue
			}

			keyVal, ok := v["key"].(string)
			if !ok || strings.TrimSpace(keyVal) == "" {
				s.enabled = false
				return fmt.Errorf("bitwarden secret config for %s must include 'id' or non-empty 'key'", envVar)
			}

			projectID, _ := v["project_id"].(string)
			projectID = strings.TrimSpace(projectID)
			entry := bitwardenSecretConfig{envVar: envVar, key: strings.TrimSpace(keyVal), projectID: projectID}
			keyEntries[projectID] = append(keyEntries[projectID], entry)
		default:
			s.enabled = false
			return fmt.Errorf("bitwarden secret config for %s must be a string or object", envVar)
		}
	}

	if _, err := lookupBinary("bws"); err != nil {
		s.enabled = false
		return fmt.Errorf("bitwarden CLI 'bws' not found: %w", err)
	}

	s.byID = idEntries
	s.byKey = keyEntries
	s.enabled = true
	return nil
}

func (s *Bitwarden) GetAllSecrets(previous *secret.Secrets) (*secret.Secrets, error) {
	results := secret.New()
	env := buildCommandEnv(previous)

	for _, entry := range s.byID {
		output, err := runCLICommand("bws", env, "secret", "get", entry.id)
		if err != nil {
			return nil, err
		}

		var payload bitwardenSecret
		if err := json.Unmarshal(output, &payload); err != nil {
			return nil, fmt.Errorf("failed to parse bitwarden secret %s: %w", entry.id, err)
		}

		results.Entries[entry.envVar] = strings.TrimSpace(payload.Value)
	}

	for projectID, entries := range s.byKey {
		args := []string{"secret", "list"}
		if projectID != "" {
			args = append(args, projectID)
		}

		output, err := runCLICommand("bws", env, args...)
		if err != nil {
			return nil, err
		}

		var payload []bitwardenSecret
		if err := json.Unmarshal(output, &payload); err != nil {
			return nil, fmt.Errorf("failed to parse bitwarden secret list: %w", err)
		}

		secretsByKey := make(map[string]string, len(payload))
		for _, item := range payload {
			secretsByKey[item.Key] = item.Value
		}

		for _, entry := range entries {
			value, ok := secretsByKey[entry.key]
			if !ok {
				return nil, fmt.Errorf("bitwarden secret with key %s not found", entry.key)
			}
			results.Entries[entry.envVar] = strings.TrimSpace(value)
		}
	}

	return results, nil
}

func (s *Bitwarden) IsEnabled() bool {
	return s.enabled
}
