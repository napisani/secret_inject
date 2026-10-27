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

type bitwardenConfig struct {
	Secrets map[string]bitwardenRef `json:"secrets"`
}

// bitwardenRef is either a bare secret ID string or an object naming the
// secret by "id", or by "key" within an optional "project_id".
type bitwardenRef struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	ProjectID string `json:"project_id"`
	bare      bool
}

func (r *bitwardenRef) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		r.bare = true
		return json.Unmarshal(data, &r.ID)
	}
	type fields bitwardenRef
	if err := json.Unmarshal(data, (*fields)(r)); err != nil {
		return fmt.Errorf("must be a string or object: %w", err)
	}
	return nil
}

type Bitwarden struct {
	command providerCommand
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

func (s *Bitwarden) Init(raw json.RawMessage) error {
	s.enabled = false
	var bwConfig bitwardenConfig
	if configured, err := decodeSourceConfig(raw, &bwConfig); err != nil || !configured {
		return err
	}

	if len(bwConfig.Secrets) == 0 {
		return fmt.Errorf("bitwarden source requires a non-empty 'secrets' map")
	}

	var idEntries []bitwardenSecretConfig
	keyEntries := make(map[string][]bitwardenSecretConfig)

	for envVar, ref := range bwConfig.Secrets {
		id := strings.TrimSpace(ref.ID)
		key := strings.TrimSpace(ref.Key)
		switch {
		case id != "":
			idEntries = append(idEntries, bitwardenSecretConfig{envVar: envVar, id: id})
		case ref.bare:
			return fmt.Errorf("bitwarden secret id for %s cannot be empty", envVar)
		case key == "":
			return fmt.Errorf("bitwarden secret config for %s must include 'id' or non-empty 'key'", envVar)
		default:
			projectID := strings.TrimSpace(ref.ProjectID)
			entry := bitwardenSecretConfig{envVar: envVar, key: key, projectID: projectID}
			keyEntries[projectID] = append(keyEntries[projectID], entry)
		}
	}

	if _, err := lookupBinary("bws"); err != nil {
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
		output, err := sourceCommand(s.command)(
			"bws",
			env,
			"secret",
			"get",
			entry.id,
		)
		if err != nil {
			return nil, err
		}

		var payload bitwardenSecret
		if err := json.Unmarshal(output, &payload); err != nil {
			return nil, &ProviderError{Provider: "bitwarden", Operation: "get", Kind: "parse", ExitCode: -1}
		}

		results.Entries[entry.envVar] = strings.TrimSpace(payload.Value)
	}

	for projectID, entries := range s.byKey {
		args := []string{"secret", "list"}
		if projectID != "" {
			args = append(args, projectID)
		}

		output, err := sourceCommand(s.command)("bws", env, args...)
		if err != nil {
			return nil, err
		}

		var payload []bitwardenSecret
		if err := json.Unmarshal(output, &payload); err != nil {
			return nil, &ProviderError{Provider: "bitwarden", Operation: "list", Kind: "parse", ExitCode: -1}
		}

		secretsByKey := make(map[string]string, len(payload))
		for _, item := range payload {
			secretsByKey[item.Key] = item.Value
		}

		for _, entry := range entries {
			value, ok := secretsByKey[entry.key]
			if !ok {
				return nil, &ProviderError{Provider: "bitwarden", Operation: "list", Kind: "missing-secret", ExitCode: -1}
			}
			results.Entries[entry.envVar] = strings.TrimSpace(value)
		}
	}

	return results, nil
}

func (s *Bitwarden) IsEnabled() bool {
	return s.enabled
}
