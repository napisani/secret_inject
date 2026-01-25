package source

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/napisani/secret_inject/internal/secret"
)

type DopplerConfig struct {
	Project string `json:"project"`
	Env     string `json:"env"`
}

type Doppler struct {
	config  *DopplerConfig
	enabled bool
}

func init() {
	registerSource("doppler", func() Source { return NewDoppler() })
}

func NewDoppler() *Doppler {
	return &Doppler{
		enabled: false,
	}
}

func (s *Doppler) Init(fullConfig map[string]interface{}) error {
	sources, ok := fullConfig["sources"].(map[string]interface{})
	if !ok {
		s.enabled = false
		return nil
	}
	rawDopplerConfig, ok := sources["doppler"].(map[string]interface{})
	if !ok {
		s.enabled = false
		return nil
	}

	s.enabled = true
	dopplerConfig := DopplerConfig{}

	project, ok := rawDopplerConfig["project"].(string)
	if !ok || project == "" {
		s.enabled = false
		return fmt.Errorf("doppler source requires 'project' field")
	}
	dopplerConfig.Project = project

	env, ok := rawDopplerConfig["env"].(string)
	if !ok || env == "" {
		s.enabled = false
		return fmt.Errorf("doppler source requires 'env' field")
	}
	dopplerConfig.Env = env

	if _, err := lookupBinary("doppler"); err != nil {
		s.enabled = false
		return fmt.Errorf("doppler CLI not found: %w", err)
	}

	s.config = &dopplerConfig
	return nil
}

func (s *Doppler) GetAllSecrets(previous *secret.Secrets) (*secret.Secrets, error) {
	env := buildCommandEnv(previous)
	output, err := runCLICommand("doppler", env, "--project", s.config.Project, "--json", "secrets", "--config", s.config.Env)
	if err != nil {
		return nil, err
	}

	var dopplerOutput map[string]interface{}
	if err := json.Unmarshal(output, &dopplerOutput); err != nil {
		return nil, err
	}

	secrets := secret.New()
	for key, value := range dopplerOutput {
		entry, ok := value.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("unexpected doppler secret format for %s", key)
		}

		computed, ok := entry["computed"].(string)
		if !ok {
			return nil, fmt.Errorf("doppler secret missing computed field for %s", key)
		}

		secrets.Entries[key] = strings.TrimSpace(computed)
	}

	return secrets, nil
}

func (s *Doppler) IsEnabled() bool {
	return s.enabled
}
