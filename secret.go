package main

import (
	"encoding/json"
	"log/slog"
)

type Secrets struct {
	Entries map[string]string `json:"entries"`
}

func (s *Secrets) Serialize() ([]byte, error) {
	jsonBytes, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	slog.Debug("Serialized secrets: %s", jsonBytes)
	return jsonBytes, nil
}

func Deserialize(jsonBytes []byte) (*Secrets, error) {
	var secrets Secrets
	err := json.Unmarshal(jsonBytes, &secrets)
	if err != nil {
		return nil, err
	}
  slog.Debug("Deserialized secrets: %s", secrets)
	return &secrets, nil
}

func NewSecrets() *Secrets {
	return &Secrets{
		Entries: make(map[string]string),
	}
}

func (s *Secrets) AppendSecrets(other *Secrets) *Secrets {
	secrets := NewSecrets()
	for key, value := range s.Entries {
		secrets.Entries[key] = value
	}
	for key, value := range other.Entries {
		secrets.Entries[key] = value
	}
	return secrets
}

