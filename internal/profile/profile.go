package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/napisani/secret_inject/internal/envfile"
	"github.com/napisani/secret_inject/internal/secret"
)

type Profiles map[string]Profile

type Profile struct {
	mode          string
	rules         []string
	overwrite     bool
	rename        map[string]string
	remoteSources bool
	envFiles      []string
}

type Event struct {
	Kind        string
	Mode        string
	Rule        string
	Original    string
	Reason      string
	Destination string
	Winner      string
	Discarded   string
}

type MissingReport struct {
	Profile         string   `json:"profile"`
	MissingIncludes []string `json:"missing_includes"`
}

func object(data []byte) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("expected an object")
	}
	fields := make(map[string]json.RawMessage)
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return nil, errors.New("invalid object")
		}
		name, ok := token.(string)
		if !ok {
			return nil, errors.New("invalid object key")
		}
		if _, exists := fields[name]; exists {
			return nil, errors.New("duplicate profile field or name")
		}
		var raw json.RawMessage
		if err := d.Decode(&raw); err != nil {
			return nil, errors.New("invalid profile field")
		}
		fields[name] = raw
	}
	if _, err := d.Token(); err != nil {
		return nil, errors.New("invalid object ending")
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errors.New("unexpected trailing data")
	}
	return fields, nil
}

func (ps *Profiles) UnmarshalJSON(data []byte) error {
	fields, err := object(data)
	if err != nil {
		return err
	}
	parsed := make(Profiles, len(fields))
	for name, raw := range fields {
		if name == "" || strings.TrimSpace(name) != name {
			return errors.New("profile name must be nonempty without surrounding whitespace")
		}
		var p Profile
		if err := json.Unmarshal(raw, &p); err != nil {
			return fmt.Errorf("profile %q: %w", name, err)
		}
		parsed[name] = p
	}
	*ps = parsed
	return nil
}

func (p *Profile) UnmarshalJSON(data []byte) error {
	fields, err := object(data)
	if err != nil {
		return err
	}
	parsed := Profile{overwrite: true, rename: map[string]string{}, remoteSources: true}
	for key, raw := range fields {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return errors.New("profile fields cannot be null")
		}
		switch key {
		case "include", "exclude":
			if parsed.mode != "" {
				return errors.New("profile requires exactly one include or exclude array")
			}
			parsed.mode = key
			if err := json.Unmarshal(raw, &parsed.rules); err != nil {
				return errors.New("profile rules must be a string array")
			}
		case "overwrite":
			if err := json.Unmarshal(raw, &parsed.overwrite); err != nil {
				return errors.New("overwrite must be boolean")
			}
		case "remote_sources":
			if err := json.Unmarshal(raw, &parsed.remoteSources); err != nil {
				return errors.New("remote_sources must be boolean")
			}
		case "env_files":
			if err := json.Unmarshal(raw, &parsed.envFiles); err != nil {
				return errors.New("env_files must be a string array")
			}
			if err := envfile.ValidatePatterns(parsed.envFiles); err != nil {
				return err
			}
		case "rename":
			parsed.rename, err = decodeRenames(raw)
			if err != nil {
				return err
			}
		default:
			return errors.New("unknown profile field")
		}
	}
	if parsed.mode == "" {
		return errors.New("profile requires exactly one include or exclude array")
	}
	parsed.rules, err = validateRules(parsed.rules)
	if err != nil {
		return err
	}
	*p = parsed
	return nil
}

func decodeRenames(data []byte) (map[string]string, error) {
	mappings, err := object(data)
	if err != nil {
		return nil, errors.New("rename must be an object")
	}
	renames := make(map[string]string, len(mappings))
	targets := make(map[string]bool, len(mappings))
	for original, value := range mappings {
		var destination string
		if original == "" {
			return nil, errors.New("invalid rename name or destination")
		}
		if err := json.Unmarshal(value, &destination); err != nil {
			return nil, errors.New("invalid rename name or destination")
		}
		if !ValidName(destination) {
			return nil, errors.New("invalid rename name or destination")
		}
		if targets[destination] {
			return nil, errors.New("duplicate rename destination")
		}
		targets[destination] = true
		renames[original] = destination
	}
	return renames, nil
}

func validateRules(input []string) ([]string, error) {
	unique := make(map[string]bool, len(input))
	rules := make([]string, 0, len(input))
	for _, rule := range input {
		if rule == "" {
			return nil, errors.New("empty profile rule")
		}
		for _, c := range rule {
			if c != '*' && !isNameCharacter(c) {
				return nil, errors.New("only identifier characters and * are allowed in rules")
			}
		}
		if !strings.Contains(rule, "*") && !ValidName(rule) {
			return nil, errors.New("invalid literal variable name")
		}
		if !unique[rule] {
			unique[rule] = true
			rules = append(rules, rule)
		}
	}
	return rules, nil
}

// Choose returns nil for a profile-free legacy export when no name is supplied.
func (ps Profiles) Choose(name *string) (*Profile, error) {
	if name == nil {
		if len(ps) > 0 {
			return nil, errors.New("--profile is required when profiles are configured")
		}
		return nil, nil
	}
	p, ok := ps[*name]
	if !ok {
		return nil, errors.New("unknown or empty profile name")
	}
	return &p, nil
}

func (p *Profile) Overwrite() bool { return p == nil || p.overwrite }

func (p *Profile) RemoteSources() bool { return p == nil || p.remoteSources }

func (p *Profile) EnvFiles() []string {
	if p == nil {
		return nil
	}
	return p.envFiles
}

func ValidName(name string) bool {
	if name == "" {
		return false
	}
	startsWithDigit := name[0] >= '0' && name[0] <= '9'
	if startsWithDigit {
		return false
	}
	for _, c := range name {
		if !isNameCharacter(c) {
			return false
		}
	}
	return true
}

func isNameCharacter(c rune) bool {
	isLower := c >= 'a' && c <= 'z'
	isUpper := c >= 'A' && c <= 'Z'
	isDigit := c >= '0' && c <= '9'
	return c == '_' || isLower || isUpper || isDigit
}

func match(rule, name string) bool {
	r, n, star, retry := 0, 0, -1, 0
	for n < len(name) {
		if r < len(rule) && rule[r] != '*' && rule[r] == name[n] {
			r++
			n++
			continue
		}
		if r < len(rule) && rule[r] == '*' {
			star = r
			r++
			retry = n
			continue
		}
		if star < 0 {
			return false
		}
		retry++
		n = retry
		r = star + 1
	}
	for r < len(rule) && rule[r] == '*' {
		r++
	}
	return r == len(rule)
}

func (p *Profile) analyze(s *secret.Secrets) (map[string]bool, []Event) {
	selected := map[string]bool{}
	events := []Event{}
	for key := range s.Entries {
		selected[key] = p == nil || p.mode == "exclude"
	}
	if p == nil {
		return selected, events
	}
	for _, rule := range p.rules {
		found := false
		for key := range s.Entries {
			if match(rule, key) {
				found = true
				selected[key] = p.mode == "include"
			}
		}
		if !found {
			events = append(events, Event{Kind: "unmatched", Mode: p.mode, Rule: rule})
		}
	}
	return selected, events
}

func (p *Profile) Check(name string, s *secret.Secrets) (MissingReport, []Event) {
	_, events := p.analyze(s)
	report := MissingReport{Profile: name, MissingIncludes: []string{}}
	for _, event := range events {
		if event.Mode == "include" {
			report.MissingIncludes = append(report.MissingIncludes, event.Rule)
		}
	}
	sort.Strings(report.MissingIncludes)
	return report, events
}

// Project preserves the input snapshot and timestamp; a nil profile selects all keys.
func (p *Profile) Project(s *secret.Secrets) (*secret.Secrets, []Event, error) {
	result := &secret.Secrets{Entries: map[string]string{}, Timestamp: s.Timestamp}
	selected, events := p.analyze(s)
	origins := map[string]string{}
	keys := sortedKeys(s.Entries)
	for _, key := range keys {
		if selected[key] {
			result.Entries[key] = s.Entries[key]
			origins[key] = key
		}
	}
	if p != nil {
		renameKeys := sortedKeys(p.rename)
		// Remove origins first so chains and cycles always read original values.
		for _, key := range renameKeys {
			destination := p.rename[key]
			if !selected[key] {
				reason := "excluded"
				if _, exists := s.Entries[key]; !exists {
					reason = "absent"
				}
				events = append(events, Event{Kind: "rename-skipped", Original: key, Reason: reason})
				continue
			}
			if destination != key {
				delete(result.Entries, key)
				delete(origins, key)
			}
		}
		for _, key := range renameKeys {
			destination := p.rename[key]
			if !selected[key] || destination == key {
				continue
			}
			if previous, exists := origins[destination]; exists {
				events = append(events, Event{
					Kind:        "rename-collision",
					Destination: destination,
					Winner:      key,
					Discarded:   previous,
				})
			}
			result.Entries[destination] = s.Entries[key]
			origins[destination] = key
		}
	}
	for _, key := range sortedKeys(result.Entries) {
		if !ValidName(key) {
			return nil, events, errors.New("selected destination is not a valid environment variable name")
		}
	}
	return result, events, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
