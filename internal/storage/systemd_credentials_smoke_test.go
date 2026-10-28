//go:build linux && systemdsmoke

package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSystemdCredentialsSyntheticHost(t *testing.T) {
	if os.Getenv("SECRET_INJECT_SYSTEMD_SMOKE") != "synthetic-only" {
		t.Fatal("explicit synthetic systemd smoke authorization is required")
	}
	native, err := openSystemdCredentials(t.Context(), filepath.Join(t.TempDir(), "records"), InteractiveSetup)
	if err != nil {
		t.Fatal(err)
	}
	checkSystemdRecordLifecycle(t, native.(*systemdCredentials))
}
