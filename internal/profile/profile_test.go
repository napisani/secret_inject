package profile

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/napisani/secret_inject/internal/secret"
)

func TestRulesRenamingAndInspection(t *testing.T) {
	var ps Profiles
	err := json.Unmarshal([]byte(`{"task":{"include":["APP_*", "API_KEY", "MISSING", "MISSING", "EMPTY"],"rename":{"APP_TOKEN":"API_KEY", "UNUSED":"OTHER"},"overwrite":false}}`), &ps)
	if err != nil {
		t.Fatal(err)
	}
	p := ps["task"]
	s := secret.New()
	s.Entries = map[string]string{"APP_TOKEN": "winner", "API_KEY": "loser", "APP_URL": "url", "EMPTY": "", "UNRELATED": "hidden"}
	got, events, err := p.Project(s)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"API_KEY": "winner", "APP_URL": "url", "EMPTY": ""}
	if !reflect.DeepEqual(got.Entries, want) {
		t.Fatal("selection or rename precedence incorrect")
	}
	if p.Overwrite() {
		t.Fatal("overwrite=false ignored")
	}
	report, _ := p.Check("task", s)
	if !reflect.DeepEqual(report.MissingIncludes, []string{"MISSING"}) {
		t.Fatalf("report: %+v", report)
	}
	kinds := map[string]bool{}
	for _, event := range events {
		kinds[event.Kind] = true
	}
	for _, kind := range []string{"unmatched", "rename-skipped", "rename-collision"} {
		if !kinds[kind] {
			t.Fatalf("missing event %s", kind)
		}
	}
}

func TestProfileValidation(t *testing.T) {
	for _, doc := range []string{
		`null`, `{"x":null}`, `{" x":{"include":[]}}`, `{"x":{}}`,
		`{"x":{"include":[],"exclude":[]}}`, `{"x":{"include":null}}`,
		`{"x":{"include":[1]}}`, `{"x":{"include":[null]}}`, `{"x":{"include":[""]}}`,
		`{"x":{"include":["A?"]}}`, `{"x":{"include":["A-[B]"]}}`,
		`{"x":{"include":[],"overwite":false}}`, `{"x":{"include":[],"overwrite":null}}`,
		`{"x":{"include":[],"rename":{"A":"X", "B":"X"}}}`,
		`{"x":{"include":[],"rename":{"A":"bad-name"}}}`,
		`{"x":{"include":[],"rename":{"":"X"}}}`,
		`{"x":{"include":[],"rename":{"A":null}}}`,
		`{"x":{"include":[],"rename":{"A":"X","A":"Y"}}}`,
		`{"x":{"include":[],"include":[]}}`,
		`{"x":{"include":[]},"x":{"exclude":[]}}`,
	} {
		var ps Profiles
		if err := json.Unmarshal([]byte(doc), &ps); err == nil {
			t.Errorf("accepted %s", doc)
		}
	}
}

func TestStarRulesAndEmptySelections(t *testing.T) {
	cases := []struct {
		rule string
		want []string
	}{
		{"*", []string{"APP_A", "APP_TOKEN", "TOKEN", "X_TOKEN", "app_a"}},
		{"APP_*", []string{"APP_A", "APP_TOKEN"}},
		{"*TOKEN", []string{"APP_TOKEN", "TOKEN", "X_TOKEN"}},
		{"A**A", []string{"APP_A"}},
		{"APP_**TOKEN", []string{"APP_TOKEN"}},
	}
	for _, tc := range cases {
		t.Run(tc.rule, func(t *testing.T) {
			var p Profile
			data, _ := json.Marshal(map[string]any{"include": []string{tc.rule}})
			if err := json.Unmarshal(data, &p); err != nil {
				t.Fatal(err)
			}
			s := secret.New()
			for _, key := range []string{"APP_A", "APP_TOKEN", "TOKEN", "X_TOKEN", "app_a"} {
				s.Entries[key] = ""
			}
			got, _, err := p.Project(s)
			if err != nil || !reflect.DeepEqual(sortedKeys(got.Entries), tc.want) {
				t.Fatalf("keys: %v, err %v", sortedKeys(got.Entries), err)
			}
		})
	}
	for _, mode := range []string{"include", "exclude"} {
		var p Profile
		if err := json.Unmarshal([]byte(`{"`+mode+`":[]}`), &p); err != nil {
			t.Fatal(err)
		}
		s := secret.New()
		s.Entries["A"] = ""
		got, _, err := p.Project(s)
		if err != nil || (len(got.Entries) == 0) != (mode == "include") {
			t.Fatal("empty rules semantics")
		}
	}
}

func TestRenamesAreExactAndNotChained(t *testing.T) {
	for _, tc := range []struct {
		doc         string
		input, want map[string]string
	}{
		{`{"include":["*"],"rename":{"A":"B","B":"C"}}`, map[string]string{"A": "first", "B": "second", "C": "discarded"}, map[string]string{"B": "first", "C": "second"}},
		{`{"include":["*"],"rename":{"A":"B","B":"A"}}`, map[string]string{"A": "first", "B": "second"}, map[string]string{"A": "second", "B": "first"}},
		{`{"include":["*"],"rename":{"ODD*KEY":"SAFE"}}`, map[string]string{"ODD*KEY": "literal", "ODD_OTHER_KEY": "unchanged"}, map[string]string{"SAFE": "literal", "ODD_OTHER_KEY": "unchanged"}},
	} {
		var p Profile
		if err := json.Unmarshal([]byte(tc.doc), &p); err != nil {
			t.Fatal(err)
		}
		s := secret.New()
		s.Entries = tc.input
		got, _, err := p.Project(s)
		if err != nil || !reflect.DeepEqual(got.Entries, tc.want) {
			t.Fatal("rename was expanded or chained")
		}
	}
}

func TestSelectedNamesAreValidatedAfterRename(t *testing.T) {
	var p Profile
	if err := json.Unmarshal([]byte(`{"include":["*"],"rename":{"unsafe-name":"SAFE"}}`), &p); err != nil {
		t.Fatal(err)
	}
	s := secret.New()
	s.Entries["unsafe-name"] = "value"
	if _, _, err := p.Project(s); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (*Profile)(nil).Project(s); err == nil {
		t.Fatal("legacy output accepted invalid name")
	}
}

func TestProfilesRequireExplicitChoice(t *testing.T) {
	var profiles Profiles
	if err := json.Unmarshal([]byte(`{"shell":{"exclude":["OP_TOKEN"]}}`), &profiles); err != nil {
		t.Fatal(err)
	}
	if _, err := profiles.Choose(nil); err == nil {
		t.Fatal("profile must be explicit")
	}
	name := "missing"
	if _, err := profiles.Choose(&name); err == nil {
		t.Fatal("unknown profile accepted")
	}
	name = "shell"
	p, err := profiles.Choose(&name)
	if err != nil || !p.Overwrite() {
		t.Fatalf("default overwrite: %v", err)
	}
	s := secret.New()
	s.Entries = map[string]string{"OP_TOKEN": "bootstrap", "API_KEY": "value"}
	projected, _, err := p.Project(s)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(projected.Entries, map[string]string{"API_KEY": "value"}) {
		t.Fatalf("unexpected projection keys")
	}
	if len(s.Entries) != 2 || !projected.Timestamp.Equal(s.Timestamp) {
		t.Fatal("projection mutated original snapshot")
	}
}
