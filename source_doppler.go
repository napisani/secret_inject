package main

import (
	"encoding/json"
	"os"
	"os/exec"
)

type SourceDopplerConfig struct {
	Project string `json:"project"`
	Env     string `json:"env"`
}

type SourceDoppler struct {
	config  *SourceDopplerConfig
	Enabled bool
}

func NewSourceDoppler() *SourceDoppler {
	return &SourceDoppler{
		Enabled: false,
	}
}

func (s *SourceDoppler) Init(fullConfig map[string]interface{}) error {
	sources, ok := fullConfig["sources"].(map[string]interface{})
	if !ok {
		s.Enabled = false
		return nil
	}
	rawDopplerConfig, ok := sources["doppler"].(map[string]interface{})
	if !ok {
		s.Enabled = false
		return nil
	}

	s.Enabled = true
	dopplerConfig := SourceDopplerConfig{}
	dopplerConfig.Project = rawDopplerConfig["project"].(string)
	dopplerConfig.Env = rawDopplerConfig["env"].(string)

	s.config = &dopplerConfig
	return nil
}

func (s *SourceDoppler) GetAllSecrets() (*Secrets, error) {
	MkdirRecursive(os.TempDir())
	file, err := CreateFileIfNotExist(fullFilePath)
	defer file.Close()

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

	secrets := NewSecrets()
	for key, value := range dopplerOutput {
		secret := value.(map[string]interface{})["computed"].(string)
		secrets.Entries[key] = secret
	}

	return secrets, nil
}
