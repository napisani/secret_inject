package source

import (
	"fmt"
	"sort"
	"strings"

	"github.com/napisani/secret_inject/internal/secret"
)

type Source interface {
	Init(config map[string]interface{}) error
	GetAllSecrets(previous *secret.Secrets) (*secret.Secrets, error)
	IsEnabled() bool
}

var sourceRegistry = map[string]func() Source{}

func registerSource(name string, factory func() Source) {
	if _, exists := sourceRegistry[name]; exists {
		panic(fmt.Sprintf("source %s already registered", name))
	}
	sourceRegistry[name] = factory
}

func resolveSourceOrder(fullConfig map[string]interface{}, sourcesConfig map[string]interface{}) ([]string, error) {
	sequenceRaw, hasSequence := fullConfig["source_sequence"]
	if !hasSequence {
		keys := make([]string, 0, len(sourcesConfig))
		for key := range sourcesConfig {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return keys, nil
	}

	sequenceSlice, ok := sequenceRaw.([]interface{})
	if !ok {
		if sequenceStrings, ok := sequenceRaw.([]string); ok {
			sequenceSlice = make([]interface{}, 0, len(sequenceStrings))
			for _, name := range sequenceStrings {
				sequenceSlice = append(sequenceSlice, name)
			}
		} else {
			return nil, fmt.Errorf("source_sequence must be an array of strings")
		}
	}

	ordered := make([]string, 0, len(sourcesConfig))
	seen := make(map[string]bool, len(sourcesConfig))
	for _, entry := range sequenceSlice {
		name, ok := entry.(string)
		if !ok || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("source_sequence entries must be non-empty strings")
		}
		if _, exists := sourcesConfig[name]; !exists {
			return nil, fmt.Errorf("source_sequence references unknown source %q", name)
		}
		if !seen[name] {
			ordered = append(ordered, name)
			seen[name] = true
		}
	}

	remaining := make([]string, 0)
	for key := range sourcesConfig {
		if !seen[key] {
			remaining = append(remaining, key)
		}
	}
	sort.Strings(remaining)
	ordered = append(ordered, remaining...)

	return ordered, nil
}

func LoadAll(fullConfig map[string]interface{}) ([]Source, error) {
	sourcesConfigRaw, _ := fullConfig["sources"].(map[string]interface{})
	if len(sourcesConfigRaw) == 0 {
		return nil, nil
	}

	keys, err := resolveSourceOrder(fullConfig, sourcesConfigRaw)
	if err != nil {
		return nil, err
	}

	loaded := make([]Source, 0, len(keys))
	for _, key := range keys {
		factory, ok := sourceRegistry[key]
		if !ok {
			return nil, fmt.Errorf("unknown source %q", key)
		}

		instance := factory()
		if err := instance.Init(fullConfig); err != nil {
			return nil, fmt.Errorf("initializing source %s: %w", key, err)
		}

		if instance.IsEnabled() {
			loaded = append(loaded, instance)
		}
	}

	return loaded, nil
}
