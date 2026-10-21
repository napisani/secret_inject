package source

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/napisani/secret_inject/internal/secret"
)

// Init receives this source's own raw config (the value of sources[<name>] in
// the top-level config), which is nil or JSON null when the source isn't
// configured. Implementations disable themselves in that case and return an
// error for a config of the wrong shape.
type Source interface {
	Init(config json.RawMessage) error
	GetAllSecrets(previous *secret.Secrets) (*secret.Secrets, error)
	IsEnabled() bool
}

var sourceRegistry = map[string]func() Source{}

// decodeSourceConfig decodes raw into dst. It reports false, with no error,
// when the source is unconfigured (nil or JSON null).
func decodeSourceConfig(raw json.RawMessage, dst any) (bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return false, nil
	}
	if err := json.Unmarshal(trimmed, dst); err != nil {
		return false, fmt.Errorf("invalid config: %w", err)
	}
	return true, nil
}

func registerSource(name string, factory func() Source) {
	if _, exists := sourceRegistry[name]; exists {
		panic(fmt.Sprintf("source %s already registered", name))
	}
	sourceRegistry[name] = factory
}

// ResolveSourceOrder orders sourcesConfig's keys by sequence, appending any
// keys sequence omits (sorted) at the end. An empty sequence is equivalent to
// omitting it: everything falls into the sorted "remaining" set.
func ResolveSourceOrder(sequence []string, sourcesConfig map[string]json.RawMessage) ([]string, error) {
	ordered := make([]string, 0, len(sourcesConfig))
	seen := make(map[string]bool, len(sourcesConfig))
	for _, name := range sequence {
		if strings.TrimSpace(name) == "" {
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

// LoadAll instantiates and initializes every configured source, in the order
// given by sequence (any sources sequence omits are appended, sorted).
func LoadAll(sourcesConfig map[string]json.RawMessage, sequence []string) ([]Source, error) {
	if len(sourcesConfig) == 0 {
		return nil, nil
	}

	keys, err := ResolveSourceOrder(sequence, sourcesConfig)
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
		if err := instance.Init(sourcesConfig[key]); err != nil {
			return nil, fmt.Errorf("initializing source %s: %w", key, err)
		}

		if instance.IsEnabled() {
			loaded = append(loaded, instance)
		}
	}

	return loaded, nil
}
