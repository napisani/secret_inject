package source

import (
	"encoding/json"
	"errors"

	"github.com/napisani/secret_inject/internal/jsonobject"
)

var ErrCredentialBinding = errors.New("invalid private credential binding")

type CredentialBinding interface {
	credentialBinding()
}

type StoredBinding struct {
	Name string
}

func (StoredBinding) credentialBinding() {}

type UpstreamBinding struct {
	Source string
	Key    string
}

func (UpstreamBinding) credentialBinding() {}

type SourceCredential struct {
	Provider string
	Binding  CredentialBinding
}

// PlanCredentials validates private dependencies without initializing providers.
func PlanCredentials(sourcesConfig map[string]json.RawMessage, sequence []string) ([]SourceCredential, error) {
	order, err := DeclaredSourceOrder(sourcesConfig, sequence)
	if err != nil {
		return nil, ErrCredentialBinding
	}
	plan := make([]SourceCredential, 0, len(order))
	earlier := make(map[string]bool, len(order))
	for _, provider := range order {
		fields, err := jsonobject.Decode(sourcesConfig[provider])
		if err != nil {
			return nil, ErrCredentialBinding
		}
		binding, err := decodeCredentialBinding(fields["credential"])
		if err != nil {
			return nil, err
		}
		if upstream, ok := binding.(UpstreamBinding); ok && !earlier[upstream.Source] {
			return nil, ErrCredentialBinding
		}
		plan = append(plan, SourceCredential{Provider: provider, Binding: binding})
		earlier[provider] = true
	}
	return plan, nil
}

func decodeCredentialBinding(data []byte) (CredentialBinding, error) {
	fields, err := jsonobject.Decode(data)
	if err != nil {
		return nil, ErrCredentialBinding
	}
	kind, err := bindingString(fields, "kind")
	if err != nil {
		return nil, err
	}
	switch kind {
	case "stored":
		if len(fields) != 2 {
			return nil, ErrCredentialBinding
		}
		name, err := bindingString(fields, "name")
		if err != nil || !ValidCredentialName(name) {
			return nil, ErrCredentialBinding
		}
		return StoredBinding{Name: name}, nil
	case "from_source":
		if len(fields) != 3 {
			return nil, ErrCredentialBinding
		}
		provider, err := bindingString(fields, "source")
		if err != nil {
			return nil, err
		}
		key, err := bindingString(fields, "key")
		if err != nil {
			return nil, err
		}
		return UpstreamBinding{Source: provider, Key: key}, nil
	default:
		return nil, ErrCredentialBinding
	}
}

func bindingString(fields map[string]json.RawMessage, name string) (string, error) {
	var value string
	if err := json.Unmarshal(fields[name], &value); err != nil || value == "" {
		return "", ErrCredentialBinding
	}
	return value, nil
}

func ValidCredentialName(name string) bool {
	if len(name) == 0 || len(name) > 128 {
		return false
	}
	for index, c := range name {
		alphanumeric := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
		if !alphanumeric && (index == 0 || c != '_' && c != '-' && c != '.') {
			return false
		}
	}
	return true
}
