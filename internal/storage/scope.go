package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"

	"github.com/napisani/secret_inject/internal/source"
)

// SourceScope hashes source definitions and effective order, not profiles or auth state.
func SourceScope(sources map[string]json.RawMessage, sequence []string) (string, error) {
	order, err := source.ResolveSourceOrder(sequence, sources)
	if err != nil {
		return "", err
	}
	canonical := make(map[string]any, len(sources))
	for name, raw := range sources {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return "", errors.New("invalid source configuration")
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			return "", errors.New("unexpected trailing source configuration")
		}
		canonical[name] = value
	}
	payload := struct {
		Version int            `json:"version"`
		Sources map[string]any `json:"sources"`
		Order   []string       `json:"order"`
	}{
		Version: 1,
		Sources: canonical,
		Order:   order,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", errors.New("cannot encode source scope")
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
