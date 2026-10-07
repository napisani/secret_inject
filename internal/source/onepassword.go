package source

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/napisani/secret_inject/internal/secret"
)

type onePasswordConfig struct {
	Secrets map[string]string `json:"secrets"`
}

type OnePassword struct {
	secrets map[string]string
	enabled bool
}

func init() {
	registerSource("onepassword", func() Source { return NewOnePassword() })
}

func NewOnePassword() *OnePassword {
	return &OnePassword{}
}

func (s *OnePassword) Init(raw json.RawMessage) error {
	s.enabled = false
	var opConfig onePasswordConfig
	if configured, err := decodeSourceConfig(raw, &opConfig); err != nil || !configured {
		return err
	}

	if len(opConfig.Secrets) == 0 {
		return fmt.Errorf("onepassword source requires a non-empty 'secrets' map")
	}

	secrets := make(map[string]string, len(opConfig.Secrets))
	for envVar, ref := range opConfig.Secrets {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			return fmt.Errorf("onepassword secret reference for %s must be a non-empty string", envVar)
		}
		secrets[envVar] = ref
	}

	if _, err := lookupBinary("op"); err != nil {
		return fmt.Errorf("1password CLI 'op' not found: %w", err)
	}

	s.secrets = secrets
	s.enabled = true
	return nil
}

func (s *OnePassword) GetAllSecrets(previous *secret.Secrets) (*secret.Secrets, error) {
	results := secret.New()
	env := buildCommandEnv(previous)
	for envVar, ref := range s.secrets {
		output, err := runCLICommand("op", env, "read", ref)
		if err != nil {
			return nil, err
		}
		results.Entries[envVar] = strings.TrimSpace(string(output))
	}
	return results, nil
}

func (s *OnePassword) IsEnabled() bool {
	return s.enabled
}
