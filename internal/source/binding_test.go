package source_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/napisani/secret_inject/internal/source"
)

func TestPlanCredentialsPreservesPrivateDependencyOrderWithoutProviders(t *testing.T) {
	t.Parallel()
	configs := map[string]json.RawMessage{
		"doppler":     json.RawMessage(`{"credential":{"kind":"stored","name":"work-root"}}`),
		"onepassword": json.RawMessage(`{"credential":{"kind":"from_source","source":"doppler","key":"OP_SERVICE_ACCOUNT_TOKEN"}}`),
		"bitwarden":   json.RawMessage(`null`),
	}
	got, err := source.PlanCredentials(configs, []string{"doppler", "onepassword", "bitwarden"})
	if err != nil {
		t.Fatal(err)
	}
	want := []source.SourceCredential{
		{Provider: "doppler", Binding: source.StoredBinding{Name: "work-root"}},
		{Provider: "onepassword", Binding: source.UpstreamBinding{Source: "doppler", Key: "OP_SERVICE_ACCOUNT_TOKEN"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("credential plan = %#v, want %#v", got, want)
	}
}

func TestPlanCredentialsRejectsInvalidBindings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		binding string
	}{
		{"null", `null`},
		{"array", `[]`},
		{"scalar", `"PRIVATE_BINDING_SENTINEL"`},
		{"missing kind", `{"name":"work-root"}`},
		{"unknown kind", `{"kind":"PRIVATE_BINDING_SENTINEL","name":"work-root"}`},
		{"case folded kind", `{"Kind":"stored","name":"work-root"}`},
		{"missing stored name", `{"kind":"stored"}`},
		{"empty name", `{"kind":"stored","name":""}`},
		{"null name", `{"kind":"stored","name":null}`},
		{"non-string name", `{"kind":"stored","name":42}`},
		{"path name", `{"kind":"stored","name":"../PRIVATE_BINDING_SENTINEL"}`},
		{"whitespace name", `{"kind":"stored","name":" PRIVATE_BINDING_SENTINEL"}`},
		{"invalid leading punctuation", `{"kind":"stored","name":"_work-root"}`},
		{"unicode name", `{"kind":"stored","name":"café"}`},
		{"long name", `{"kind":"stored","name":"` + strings.Repeat("x", 129) + `"}`},
		{"unknown field", `{"kind":"stored","name":"work-root","value":"PRIVATE_BINDING_SENTINEL"}`},
		{"duplicate field", `{"kind":"stored","name":"work-root","name":"PRIVATE_BINDING_SENTINEL"}`},
		{"duplicate kind", `{"kind":"stored","kind":"stored","name":"work-root"}`},
		{"missing producer", `{"kind":"from_source","key":"PRIVATE_BINDING_SENTINEL"}`},
		{"missing key", `{"kind":"from_source","source":"doppler"}`},
		{"empty key", `{"kind":"from_source","source":"doppler","key":""}`},
		{"self dependency", `{"kind":"from_source","source":"doppler","key":"PRIVATE_BINDING_SENTINEL"}`},
		{"future dependency", `{"kind":"from_source","source":"onepassword","key":"PRIVATE_BINDING_SENTINEL"}`},
		{"unknown dependency", `{"kind":"from_source","source":"PRIVATE_BINDING_SENTINEL","key":"TOKEN"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			configs := map[string]json.RawMessage{
				"doppler":     json.RawMessage(`{"credential":` + tt.binding + `}`),
				"onepassword": json.RawMessage(`{"credential":{"kind":"stored","name":"other-root"}}`),
			}
			plan, err := source.PlanCredentials(configs, []string{"doppler", "onepassword"})
			if !errors.Is(err, source.ErrCredentialBinding) || plan != nil {
				t.Fatalf("invalid binding produced plan=%#v err=%v", plan, err)
			}
			if strings.Contains(err.Error(), "PRIVATE_BINDING_SENTINEL") {
				t.Fatal("binding error exposed input")
			}
		})
	}
}

func TestPlanCredentialsRejectsInvalidSourceDeclarations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		sources  map[string]json.RawMessage
		sequence []string
	}{
		{"missing binding", map[string]json.RawMessage{"doppler": json.RawMessage(`{}`)}, nil},
		{"non-object source", map[string]json.RawMessage{"doppler": json.RawMessage(`[]`)}, nil},
		{"duplicate source field", map[string]json.RawMessage{"doppler": json.RawMessage(`{"credential":{},"credential":{}}`)}, nil},
		{"unknown provider", map[string]json.RawMessage{"PRIVATE_BINDING_SENTINEL": json.RawMessage(`{}`)}, nil},
		{"invalid order", map[string]json.RawMessage{}, []string{"PRIVATE_BINDING_SENTINEL"}},
		{"disabled producer", map[string]json.RawMessage{
			"doppler":     json.RawMessage(`null`),
			"onepassword": json.RawMessage(`{"credential":{"kind":"from_source","source":"doppler","key":"TOKEN"}}`),
		}, []string{"doppler", "onepassword"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			plan, err := source.PlanCredentials(tt.sources, tt.sequence)
			if !errors.Is(err, source.ErrCredentialBinding) || plan != nil {
				t.Fatalf("invalid source declarations produced plan=%#v err=%v", plan, err)
			}
			if strings.Contains(err.Error(), "PRIVATE_BINDING_SENTINEL") {
				t.Fatal("source declaration error exposed input")
			}
		})
	}
}

func TestPlanCredentialsAcceptsBoundedCredentialNames(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"a", "1", "work.root_1-test", strings.Repeat("x", 128)} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			configs := map[string]json.RawMessage{
				"doppler": json.RawMessage(`{"credential":{"kind":"stored","name":"` + name + `"}}`),
			}
			if _, err := source.PlanCredentials(configs, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPlanCredentialsAllowsNoEnabledSources(t *testing.T) {
	t.Parallel()
	plan, err := source.PlanCredentials(map[string]json.RawMessage{"doppler": json.RawMessage(`null`)}, nil)
	if err != nil || plan == nil || len(plan) != 0 {
		t.Fatalf("disabled plan=%#v err=%v", plan, err)
	}
}

func FuzzPlanCredentials(f *testing.F) {
	f.Add([]byte(`{"credential":{"kind":"stored","name":"work-root"}}`))
	f.Add([]byte(`{"credential":{"kind":"stored","name":"a","name":"b"}}`))
	f.Add([]byte(`{"credential":null}`))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, data []byte) {
		plan, err := source.PlanCredentials(map[string]json.RawMessage{"doppler": data}, nil)
		if err != nil && (!errors.Is(err, source.ErrCredentialBinding) || plan != nil) {
			t.Fatal("invalid input did not produce a classified failure")
		}
	})
}
