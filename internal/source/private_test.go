package source_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/napisani/secret_inject/internal/source"
)

func TestResolvePrivatePartitionsBootstrapBeforeWorkloadMerge(t *testing.T) {
	t.Parallel()
	configs := map[string]json.RawMessage{
		"doppler":     json.RawMessage(`{"credential":{"kind":"stored","name":"work-root"}}`),
		"onepassword": json.RawMessage(`{"credential":{"kind":"from_source","source":"doppler","key":"OP_SERVICE_ACCOUNT_TOKEN"}}`),
	}
	var called []string
	fetch := func(ctx context.Context, provider string, raw json.RawMessage, token string) (map[string]string, error) {
		called = append(called, provider)
		if provider == "doppler" {
			if token != "ROOT_PRIVATE_TOKEN" {
				t.Fatal("wrong stored root")
			}
			return map[string]string{"OP_SERVICE_ACCOUNT_TOKEN": "DERIVED_PRIVATE_TOKEN", "APP_KEY": "first"}, nil
		}
		if token != "DERIVED_PRIVATE_TOKEN" {
			t.Fatal("derived credential was not kept private")
		}
		return map[string]string{"APP_KEY": "last"}, nil
	}
	roots := func(name, provider string) (string, error) {
		if name != "work-root" || provider != "doppler" {
			return "", errors.New("unexpected credential")
		}
		return "ROOT_PRIVATE_TOKEN", nil
	}
	got, err := source.ResolvePrivate(t.Context(), configs, []string{"doppler", "onepassword"}, roots, fetch)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Entries, map[string]string{"APP_KEY": "last"}) || !reflect.DeepEqual(called, []string{"doppler", "onepassword"}) {
		t.Fatal("private/workload channels were mixed")
	}
}
