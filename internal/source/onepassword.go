package source

import (
	"fmt"
	"strings"

	"github.com/napisani/secret_inject/internal/secret"
)

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

func (s *OnePassword) Init(fullConfig map[string]interface{}) error {
	sources, ok := fullConfig["sources"].(map[string]interface{})
	if !ok {
		s.enabled = false
		return nil
	}

	rawConfig, ok := sources["onepassword"].(map[string]interface{})
	if !ok {
		s.enabled = false
		return nil
	}

	rawSecrets, ok := rawConfig["secrets"].(map[string]interface{})
	if !ok || len(rawSecrets) == 0 {
		s.enabled = false
		return fmt.Errorf("onepassword source requires a non-empty 'secrets' map")
	}

	secrets := make(map[string]string, len(rawSecrets))
	for envVar, value := range rawSecrets {
		ref, ok := value.(string)
		if !ok || strings.TrimSpace(ref) == "" {
			s.enabled = false
			return fmt.Errorf("onepassword secret reference for %s must be a non-empty string", envVar)
		}
		secrets[envVar] = strings.TrimSpace(ref)
	}

	if _, err := lookupBinary("op"); err != nil {
		s.enabled = false
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
