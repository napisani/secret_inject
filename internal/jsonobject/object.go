package jsonobject

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// Decode accepts one JSON object with unique, case-sensitive field names.
func Decode(data []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("expected an object")
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, errors.New("invalid object")
		}
		name, ok := token.(string)
		if !ok {
			return nil, errors.New("invalid object key")
		}
		if _, exists := fields[name]; exists {
			return nil, errors.New("duplicate object field")
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, errors.New("invalid object field")
		}
		fields[name] = raw
	}
	if _, err := decoder.Token(); err != nil {
		return nil, errors.New("invalid object ending")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("unexpected trailing data")
	}
	return fields, nil
}
