package profile

type SelectionDescription struct {
	Mode     string   `json:"mode"`
	Patterns []string `json:"patterns"`
}

type Description struct {
	Name           string               `json:"name"`
	Selection      SelectionDescription `json:"selection"`
	Rename         map[string]string    `json:"rename"`
	ShellOverwrite bool                 `json:"shell_overwrite"`
	RemoteSources  bool                 `json:"remote_sources"`
	EnvFiles       []string             `json:"env_files"`
}

// Describe projects validated policy without evaluating inputs or exposing values.
func (p Profile) Describe(name string) Description {
	renames := make(map[string]string, len(p.rename))
	for original, destination := range p.rename {
		renames[original] = destination
	}
	return Description{
		Name:           name,
		Selection:      SelectionDescription{Mode: p.mode, Patterns: append([]string{}, p.rules...)},
		Rename:         renames,
		ShellOverwrite: p.overwrite,
		RemoteSources:  p.remoteSources,
		EnvFiles:       append([]string{}, p.envFiles...),
	}
}
