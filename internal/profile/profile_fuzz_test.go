package profile

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/napisani/secret_inject/internal/secret"
)

func FuzzProfileStarMatching(f *testing.F) {
	f.Add("APP_*", "APP_TOKEN")
	f.Add("*_TOKEN", "TOKEN")
	f.Add("*A*B*", "APP_B")
	f.Add("A**A", "APP_A")
	f.Add("*", "_EMPTY")
	f.Fuzz(func(t *testing.T, rule, name string) {
		if !ValidName(name) {
			t.Skip()
		}
		data, err := json.Marshal(map[string][]string{"include": {rule}})
		if err != nil {
			t.Fatal(err)
		}
		var p Profile
		if err := json.Unmarshal(data, &p); err != nil {
			t.Skip()
		}
		expression := "^" + strings.ReplaceAll(regexp.QuoteMeta(rule), `\*`, ".*") + "$"
		expected, err := regexp.MatchString(expression, name)
		if err != nil {
			t.Fatal(err)
		}
		snapshot := secret.New()
		snapshot.Entries[name] = "synthetic"
		projected, _, err := p.Project(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if selected := len(projected.Entries) == 1; selected != expected {
			t.Fatal("profile selection disagrees with independent pattern matcher")
		}
		report, _ := p.Check("test", snapshot)
		if complete := len(report.MissingIncludes) == 0; complete != expected {
			t.Fatal("missing-rule report disagrees with selection")
		}
		projected.Entries[name] = "changed"
		if snapshot.Entries[name] != "synthetic" {
			t.Fatal("projection shares mutable entries with input snapshot")
		}
	})
}
