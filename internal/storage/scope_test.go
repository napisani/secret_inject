package storage

import (
	"encoding/json"
	"testing"
)

func TestScopePreservesNumericTokensAndRejectsTrailingData(t *testing.T) {
	a, err := SourceScope(map[string]json.RawMessage{"doppler": json.RawMessage(`{"extra":9007199254740993}`)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := SourceScope(map[string]json.RawMessage{"doppler": json.RawMessage(`{"extra":9007199254740992}`)}, nil)
	if err != nil || a == b {
		t.Fatal("scope collapsed distinct large numeric tokens")
	}
	if _, err := SourceScope(map[string]json.RawMessage{"doppler": json.RawMessage(`{} {}`)}, nil); err == nil {
		t.Fatal("trailing source data ignored")
	}
}

func TestSourceScopeCanonicalAndOrderSensitive(t *testing.T) {
	sources := func(doc string) map[string]json.RawMessage {
		t.Helper()
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(doc), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	a := sources(`{"doppler":{"project":"a","env":"dev"},"onepassword":null}`)
	b := sources(`{ "onepassword": null, "doppler": { "env":"dev", "project":"a" } }`)
	first, err := SourceScope(a, nil)
	if err != nil {
		t.Fatal(err)
	}
	equal, err := SourceScope(b, []string{"doppler", "doppler"})
	if err != nil {
		t.Fatal(err)
	}
	if first != equal || len(first) != 64 {
		t.Fatal("noncanonical scope")
	}
	changed, err := SourceScope(a, []string{"onepassword"})
	if err != nil || changed == first {
		t.Fatal("order not part of identity")
	}
	b["doppler"] = json.RawMessage(`{"project":"b","env":"dev"}`)
	changed, err = SourceScope(b, nil)
	if err != nil || changed == first {
		t.Fatal("source definition not part of identity")
	}
	empty, _ := SourceScope(nil, nil)
	emptyMap, _ := SourceScope(map[string]json.RawMessage{}, nil)
	if empty != emptyMap {
		t.Fatal("empty scope differs")
	}
	if _, err := SourceScope(a, []string{"unknown"}); err == nil {
		t.Fatal("unknown sequence accepted")
	}
}
