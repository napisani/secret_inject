package main

import (
	"encoding/json"
	"io/ioutil"
  "log/slog"
)

type SecretSource interface {
	Init(fullConfig map[string]interface{}) error

	GetAllSecrets() (*Secrets, error)
}

func ReadFullConfig(filename string) (map[string]interface{}, error) {
  slog.Debug("Reading config file: %s", filename)
	var fullConfig map[string]interface{}

	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(content, &fullConfig)
	if err != nil {
		return nil, err
	}

  slog.Debug("Read config contents: %s", fullConfig)
	return fullConfig, nil
}
