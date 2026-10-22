package profile

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestProfileInputSelection(t *testing.T) {
	var ps Profiles
	if err := json.Unmarshal([]byte(`{
		"shell":{"exclude":[]},
		"project-only":{"exclude":[],"remote_sources":false,"env_files":["*.env","*.env.local"]},
		"project":{"exclude":[],"env_files":["*.env","*.env.local"]}
	}`), &ps); err != nil {
		t.Fatal(err)
	}
	for name, p := range ps {
		if p.RemoteSources() != (name != "project-only") {
			t.Fatal("wrong remote source selection")
		}
		if name != "shell" && !reflect.DeepEqual(p.EnvFiles(), []string{"*.env", "*.env.local"}) {
			t.Fatal("local input order changed")
		}
	}
	var legacy *Profile
	if !legacy.RemoteSources() || len(legacy.EnvFiles()) != 0 {
		t.Fatal("profile-free source behavior changed")
	}
}

func TestInvalidProfileInputs(t *testing.T) {
	for _, fields := range []string{
		`"remote_sources":null`, `"remote_sources":"false"`,
		`"env_files":null`, `"env_files":"*.env"`, `"env_files":[null]`,
		`"env_files":["../*.env"]`, `"env_files":["sub/*.env"]`,
		`"env_files":["["]`,
	} {
		var p Profile
		if err := json.Unmarshal([]byte(`{"exclude":[],`+fields+`}`), &p); err == nil {
			t.Errorf("accepted invalid input policy %s", fields)
		}
	}
}
