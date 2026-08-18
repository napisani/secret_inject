package source

import (
	"fmt"
	"sort"
	"strings"

	"github.com/napisani/secret_inject/internal/secret"
)

// Init receives just this source's own config sub-map (the value of
// sources[<name>] in the top-level config), or nil if the source isn't
// configured at all. Implementations should disable themselves on nil.
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

// resolveSourceOrder orders sourcesConfig's keys by sequence, appending any
// keys sequence omits (sorted) at the end. An empty sequence is equivalent to
// omitting it: everything falls into the sorted "remaining" set.
func resolveSourceOrder(sequence []string, sourcesConfig map[string]interface{}) ([]string, error) {
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
func LoadAll(sourcesConfig map[string]interface{}, sequence []string) ([]Source, error) {
	if len(sourcesConfig) == 0 {
		return nil, nil
	}

	keys, err := resolveSourceOrder(sequence, sourcesConfig)
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
		sourceConfig, _ := sourcesConfig[key].(map[string]interface{})
		if err := instance.Init(sourceConfig); err != nil {
			return nil, fmt.Errorf("initializing source %s: %w", key, err)
		}

		if instance.IsEnabled() {
			loaded = append(loaded, instance)
		}
	}

	return loaded, nil
}
