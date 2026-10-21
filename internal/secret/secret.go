package secret

import (
	"encoding/json"
	"maps"
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
	return jsonBytes, nil
}

func Deserialize(jsonBytes []byte) (*Secrets, error) {
	var secrets Secrets
	err := json.Unmarshal(jsonBytes, &secrets)
	if err != nil {
		return nil, err
	}
	return &secrets, nil
}

func (s *Secrets) Append(other *Secrets) *Secrets {
	result := New()
	maps.Copy(result.Entries, s.Entries)
	maps.Copy(result.Entries, other.Entries)
	return result
}

func (s *Secrets) IsExpired(ttl time.Duration) bool {
	return time.Since(s.Timestamp) > ttl
}
