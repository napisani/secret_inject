package secret

import (
	"encoding/json"
	"log/slog"
	"time"
)

type Secrets struct {
	Entries   map[string]string `json:"entries"`
	Timestamp time.Time         `json:"timestamp"`
}

func New() *Secrets {
	return &Secrets{
		Entries:   make(map[string]string),
		Timestamp: time.Now(),
	}
}

func (s *Secrets) Serialize() ([]byte, error) {
	jsonBytes, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	slog.Debug("Serialized secrets", "bytes", len(jsonBytes))
	return jsonBytes, nil
}

func Deserialize(jsonBytes []byte) (*Secrets, error) {
	var secrets Secrets
	err := json.Unmarshal(jsonBytes, &secrets)
	if err != nil {
		return nil, err
	}
	slog.Debug("Deserialized secrets", "count", len(secrets.Entries))
	return &secrets, nil
}

func (s *Secrets) Append(other *Secrets) *Secrets {
	result := New()
	for key, value := range s.Entries {
		result.Entries[key] = value
	}
	for key, value := range other.Entries {
		result.Entries[key] = value
	}
	return result
}

func (s *Secrets) IsExpired(ttl time.Duration) bool {
	return time.Since(s.Timestamp) > ttl
}
