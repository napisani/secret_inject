package source

import (
	"encoding/json"
	"fmt"
	"os/exec"

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

	// Safely extract project with validation
	project, ok := rawDopplerConfig["project"].(string)
	if !ok || project == "" {
		s.enabled = false
		return fmt.Errorf("doppler source requires 'project' field")
	}
	dopplerConfig.Project = project

	// Safely extract env with validation
	env, ok := rawDopplerConfig["env"].(string)
	if !ok || env == "" {
		s.enabled = false
		return fmt.Errorf("doppler source requires 'env' field")
	}
	dopplerConfig.Env = env

	s.config = &dopplerConfig
	return nil
}

func (s *Doppler) GetAllSecrets() (*secret.Secrets, error) {
	cmd := exec.Command("doppler", "--project", s.config.Project, "--json", "secrets", "--config", s.config.Env)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var dopplerOutput map[string]interface{}
	err = json.Unmarshal(output, &dopplerOutput)
	if err != nil {
		return nil, err
	}

	secrets := secret.New()
	for key, value := range dopplerOutput {
		secretValue := value.(map[string]interface{})["computed"].(string)
		secrets.Entries[key] = secretValue
	}

	return secrets, nil
}

func (s *Doppler) IsEnabled() bool {
	return s.enabled
}
