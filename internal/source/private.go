package source

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/napisani/secret_inject/internal/secret"
)

var ErrPrivateCredential = errors.New("private provider credential unavailable")
var ErrProviderVersion = errors.New("provider version output is invalid")
var ErrProviderCleanup = errors.New("private provider context cleanup failed")

type ProviderFetch func(context.Context, string, json.RawMessage, string) (map[string]string, error)

func ResolvePrivate(ctx context.Context, configs map[string]json.RawMessage, sequence []string, root func(string, string) (string, error), fetch ProviderFetch) (*secret.Secrets, error) {
	plan, err := PlanCredentials(configs, sequence)
	if err != nil {
		return nil, err
	}
	if fetch == nil {
		fetch = fetchPrivateProvider
	}
	captures := make(map[string]map[string][]string)
	for _, entry := range plan {
		if binding, ok := entry.Binding.(UpstreamBinding); ok {
			if captures[binding.Source] == nil {
				captures[binding.Source] = make(map[string][]string)
			}
			captures[binding.Source][binding.Key] = append(captures[binding.Source][binding.Key], entry.Provider)
		}
	}
	private := make(map[string]string)
	workload := secret.New()
	for _, entry := range plan {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var token string
		switch binding := entry.Binding.(type) {
		case StoredBinding:
			if root == nil {
				return nil, ErrPrivateCredential
			}
			token, err = root(binding.Name, entry.Provider)
		case UpstreamBinding:
			token = private[entry.Provider]
		default:
			return nil, ErrCredentialBinding
		}
		if err != nil {
			return nil, ErrPrivateCredential
		}
		if !ValidToken(token) {
			return nil, ErrPrivateCredential
		}
		values, err := fetch(ctx, entry.Provider, configs[entry.Provider], token)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for key, consumers := range captures[entry.Provider] {
			value, found := values[key]
			if !found || !ValidToken(value) {
				return nil, ErrPrivateCredential
			}
			for _, consumer := range consumers {
				private[consumer] = value
			}
			delete(values, key)
		}
		for key, value := range values {
			workload.Entries[key] = value
		}
	}
	return workload, nil
}

func ValidToken(value string) bool {
	return value != "" && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n")
}
