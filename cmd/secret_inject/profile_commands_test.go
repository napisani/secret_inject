package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestProfileListReadsOnlyConfiguration(t *testing.T) {
	path, root := commandFixture(t, true)
	t.Setenv("PATH", "")
	t.Setenv("SI_PROVIDER_FAIL", "1")

	code, out, stderr := invoke("profile", "list", "--config", path)
	if code != 0 || stderr != "" {
		t.Fatalf("profile list: code=%d stdout=%q stderr=%q", code, out, stderr)
	}
	assertJSONResponse(t, out, `{"schema_version":1,"profiles":["shell","task"]}`)
	for _, directory := range []string{"cache", "state", "tmp"} {
		entries, err := os.ReadDir(filepath.Join(root, directory))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("inspection wrote to %s", directory)
		}
	}
}

func TestProfileDescribeReadsOnlyDeclaredPolicy(t *testing.T) {
	path, root := commandFixture(t, true)
	t.Setenv("PATH", "")
	t.Setenv("SI_PROVIDER_FAIL", "1")
	t.Chdir(root)
	contents := `{
		"storage":{"type":"invalid","password":"PASSWORD_SENTINEL"},
		"source_sequence":["onepassword","bitwarden","doppler"],
		"sources":{
			"doppler":{"project":"PROJECT_SENTINEL","env":"ENV_SENTINEL",
				"credential":{"kind":"stored","name":"CREDENTIAL_SENTINEL"}},
			"onepassword":{"secrets":{"PRIVATE_KEY_NAME":"op://REFERENCE_SENTINEL"}},
			"bitwarden":null
		},
		"profiles":{"task":{
			"include":["SERVICE_*","API_KEY"],
			"rename":{"SERVICE_TOKEN":"API_KEY"},
			"overwrite":false,"env_files":["*.env"]
		}}
	}`
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "poison.env"), []byte("INVALID='DOTENV_SENTINEL"), 0600); err != nil {
		t.Fatal(err)
	}

	code, out, stderr := invoke("profile", "describe", "--config", path, "--profile", "task")
	if code != 0 || stderr != "" {
		t.Fatalf("profile describe: code=%d stdout=%q stderr=%q", code, out, stderr)
	}
	assertJSONResponse(t, out, `{
		"schema_version":1,
		"profile":{
			"name":"task",
			"selection":{"mode":"include","patterns":["SERVICE_*","API_KEY"]},
			"rename":{"SERVICE_TOKEN":"API_KEY"},
			"shell_overwrite":false,
			"remote_sources":true,
			"env_files":["*.env"],
			"declared_source_order":["onepassword","doppler"],
			"resolution":{"kind":"not_evaluated"}
		}
	}`)
	for _, sentinel := range []string{"PASSWORD_SENTINEL", "PROJECT_SENTINEL", "ENV_SENTINEL", "CREDENTIAL_SENTINEL", "REFERENCE_SENTINEL", "PRIVATE_KEY_NAME", "DOTENV_SENTINEL"} {
		if strings.Contains(out+stderr, sentinel) {
			t.Fatalf("inspection exposed %s", sentinel)
		}
	}
}

func TestProfileInspectionKeepsUnrelatedRemoteDeclarationsUnresolved(t *testing.T) {
	path, _ := commandFixture(t, true)
	t.Setenv("PATH", "")
	contents := `{
		"storage":{"type":"invalid","password":"PASSWORD_SENTINEL"},
		"sources":{"UNKNOWN_SOURCE_SENTINEL":{}},
		"source_sequence":["ORDER_SENTINEL"],
		"profiles":{
			"local":{"exclude":[],"remote_sources":false},
			"remote":{"exclude":[]}
		}
	}`
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}

	code, out, stderr := invoke("profile", "list", "--config", path)
	if code != 0 || stderr != "" {
		t.Fatalf("list depended on remote validity: %d %q", code, stderr)
	}
	assertJSONResponse(t, out, `{"schema_version":1,"profiles":["local","remote"]}`)

	code, out, stderr = invoke("profile", "describe", "--config", path, "--profile", "local")
	if code != 0 || stderr != "" {
		t.Fatalf("local describe depended on remote validity: %d %q", code, stderr)
	}
	assertJSONResponse(t, out, `{
		"schema_version":1,
		"profile":{
			"name":"local","selection":{"mode":"exclude","patterns":[]},
			"rename":{},"shell_overwrite":true,"remote_sources":false,
			"env_files":[],"declared_source_order":[],
			"resolution":{"kind":"not_evaluated"}
		}
	}`)

	code, out, stderr = invoke("profile", "describe", "--config", path, "--profile", "remote")
	if code != 1 || out != "" {
		t.Fatalf("remote describe accepted invalid declarations: %d %q", code, out)
	}
	assertJSONResponse(t, stderr, `{"schema_version":1,"error":{
		"code":"configuration","message":"configuration could not be read or validated"
	}}`)
	for _, sentinel := range []string{"UNKNOWN_SOURCE_SENTINEL", "ORDER_SENTINEL", "PASSWORD_SENTINEL"} {
		if strings.Contains(out+stderr, sentinel) {
			t.Fatal("inspection error exposed configuration")
		}
	}
}

func TestProfileListDoesNotInventLegacyProfiles(t *testing.T) {
	path, _ := commandFixture(t, false)
	t.Setenv("PATH", "")
	code, out, stderr := invoke("profile", "list", "--config", path)
	if code != 0 || stderr != "" {
		t.Fatalf("legacy list: %d %q", code, stderr)
	}
	assertJSONResponse(t, out, `{"schema_version":1,"profiles":[]}`)

	code, out, stderr = invoke("profile", "describe", "--config", path, "--profile", "default")
	if code != 1 || out != "" {
		t.Fatal("description invented a legacy default profile")
	}
	assertJSONResponse(t, stderr, `{"schema_version":1,"error":{
		"code":"profile_selection","message":"profile is unavailable"
	}}`)
}

func TestProfileInspectionUsageErrorsAreSafeJSON(t *testing.T) {
	path, _ := commandFixture(t, true)
	tests := []struct {
		name string
		args []string
	}{
		{"missing profile", []string{"profile", "describe", "--config", path}},
		{"empty profile", []string{"profile", "describe", "--config", path, "--profile", ""}},
		{"profile on list", []string{"profile", "list", "--config", path, "--profile", "ARGUMENT_SENTINEL"}},
		{"fetch flag", []string{"profile", "describe", "--config", path, "--profile", "task", "--force"}},
		{"debug flag", []string{"profile", "list", "--config", path, "--debug"}},
		{"positional argument", []string{"profile", "list", "--config", path, "ARGUMENT_SENTINEL"}},
		{"missing flag value", []string{"profile", "list", "--config"}},
		{"unknown flag", []string{"profile", "list", "--ARGUMENT_SENTINEL"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, stderr := invoke(tt.args...)
			if code != 2 || out != "" {
				t.Fatalf("usage code=%d stdout=%q", code, out)
			}
			assertJSONResponse(t, stderr, `{"schema_version":1,"error":{
				"code":"usage","message":"invalid invocation; use --help for usage"
			}}`)
		})
	}
}

func TestProfileInspectionHelpDoesNotReadConfiguration(t *testing.T) {
	for _, command := range []string{"list", "describe"} {
		t.Run(command, func(t *testing.T) {
			code, out, stderr := invoke("profile", command, "--config", filepath.Join(t.TempDir(), "missing.json"), "--help")
			if code != 0 || out != "" || !strings.Contains(stderr, "Usage of secret_inject profile "+command+":") {
				t.Fatalf("inspection help: code=%d stdout=%q stderr=%q", code, out, stderr)
			}
		})
	}
}

func TestProfileInspectionConfigurationErrorsAreSafeJSON(t *testing.T) {
	tests := []struct {
		name     string
		contents string
	}{
		{"missing file", ""},
		{"invalid profile", `{"profiles":{"PROFILE_SENTINEL":{"include":"VALUE_SENTINEL"}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "PATH_SENTINEL.json")
			if tt.contents != "" {
				if err := os.WriteFile(path, []byte(tt.contents), 0600); err != nil {
					t.Fatal(err)
				}
			}
			code, out, stderr := invoke("profile", "list", "--config", path)
			if code != 1 || out != "" {
				t.Fatal("invalid configuration produced a result")
			}
			assertJSONResponse(t, stderr, `{"schema_version":1,"error":{
				"code":"configuration","message":"configuration could not be read or validated"
			}}`)
		})
	}
}

func TestProfileInspectionWriterFailures(t *testing.T) {
	path, _ := commandFixture(t, true)
	for _, command := range []string{"list", "describe"} {
		t.Run(command, func(t *testing.T) {
			args := []string{"profile", command, "--config", path}
			if command == "describe" {
				args = append(args, "--profile", "task")
			}
			var stderr bytes.Buffer
			if code := runCLI(args, shortWriter{}, &stderr); code != 1 {
				t.Fatalf("short stdout write: status %d", code)
			}
			assertJSONResponse(t, stderr.String(), `{"schema_version":1,"error":{
				"code":"metadata_output","message":"metadata output failed"
			}}`)
		})
	}
	var stdout bytes.Buffer
	if code := runCLI([]string{"profile", "list", "--BAD_FLAG"}, &stdout, shortWriter{}); code != 1 || stdout.Len() != 0 {
		t.Fatal("failed error delivery reported success or produced stdout")
	}
	if code := runCLI([]string{"profile", "describe", "--help"}, &stdout, shortWriter{}); code != 1 || stdout.Len() != 0 {
		t.Fatal("failed help delivery reported success or produced stdout")
	}
}

func TestProfileDescribeRejectsUnknownProviderWithoutEcho(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	contents := `{
		"sources":{"UNKNOWN_PROVIDER_SENTINEL":{}},
		"profiles":{"remote":{"exclude":[]}}
	}`
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := invoke("profile", "describe", "--config", path, "--profile", "remote")
	if code != 1 || out != "" {
		t.Fatal("unknown provider produced a description")
	}
	assertJSONResponse(t, stderr, `{"schema_version":1,"error":{
		"code":"configuration","message":"configuration could not be read or validated"
	}}`)
}

func assertJSONResponse(t *testing.T, actual, expected string) {
	t.Helper()
	if !strings.HasSuffix(actual, "\n") {
		t.Fatal("response has no trailing newline")
	}
	var got, want any
	if err := json.Unmarshal([]byte(actual), &got); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if err := json.Unmarshal([]byte(expected), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON response:\n%s\nwant:\n%s", actual, expected)
	}
}
