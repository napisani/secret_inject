package profile_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/napisani/secret_inject/internal/profile"
)

func TestDescriptionDoesNotExposeMutablePolicy(t *testing.T) {
	t.Parallel()
	var selected profile.Profile
	if err := json.Unmarshal([]byte(`{
		"include":["SERVICE_*"],
		"rename":{"SERVICE_TOKEN":"API_KEY"},
		"env_files":["*.env"]
	}`), &selected); err != nil {
		t.Fatal(err)
	}

	description := selected.Describe("task")
	description.Selection.Patterns[0] = "OTHER_*"
	description.Rename["SERVICE_TOKEN"] = "OTHER_KEY"
	description.EnvFiles[0] = "other.env"

	want := profile.Description{
		Name:           "task",
		Selection:      profile.SelectionDescription{Mode: "include", Patterns: []string{"SERVICE_*"}},
		Rename:         map[string]string{"SERVICE_TOKEN": "API_KEY"},
		ShellOverwrite: true,
		RemoteSources:  true,
		EnvFiles:       []string{"*.env"},
	}
	if got := selected.Describe("task"); !reflect.DeepEqual(got, want) {
		t.Fatalf("description mutation changed profile policy: %#v", got)
	}
}
