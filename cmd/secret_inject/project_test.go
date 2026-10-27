package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/napisani/secret_inject/internal/config"
	"github.com/napisani/secret_inject/internal/storage"
)

func projectFixture(t *testing.T) (string, string, map[string]any) {
	t.Helper()
	path, root := commandFixture(t, true)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	profiles := cfg["profiles"].(map[string]any)
	profiles["project-only"] = map[string]any{"exclude": []string{}, "remote_sources": false, "env_files": []string{"*.env", "*.env.local"}}
	profiles["project"] = map[string]any{"exclude": []string{}, "env_files": []string{"*.env", "*.env.local"}}
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(project)
	return path, root, cfg
}

func writeProjectConfig(t *testing.T, path string, cfg map[string]any) {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func projectOutput(t *testing.T, path, profile string) map[string]string {
	t.Helper()
	code, out, stderr := invoke("--config", path, "--profile", profile, "--output", "json")
	if code != 0 || stderr != "" {
		t.Fatalf("export failed: %d %q", code, stderr)
	}
	var values map[string]string
	if err := json.Unmarshal([]byte(out), &values); err != nil {
		t.Fatal(err)
	}
	return values
}

func TestProjectOnlySkipsRemoteStorageAndPreflight(t *testing.T) {
	path, _, cfg := projectFixture(t)
	delete(cfg, "storage")
	cfg["sources"] = map[string]any{"not-a-provider": map[string]any{}}
	cfg["source_sequence"] = []string{"not-a-provider"}
	writeProjectConfig(t, path, cfg)
	t.Setenv("SI_PROVIDER_FAIL", "1")
	if values := projectOutput(t, path, "project-only"); len(values) != 0 {
		t.Fatal("missing local files did not produce empty output")
	}
	if err := os.WriteFile(".env", []byte("LOCAL=base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".env.local", []byte("LOCAL=override\n"), 0600); err != nil {
		t.Fatal(err)
	}
	values := projectOutput(t, path, "project-only")
	if len(values) != 1 || values["LOCAL"] != "override" {
		t.Fatal("project-only did not use only local inputs")
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cache, "secret_inject")); !os.IsNotExist(err) {
		t.Fatal("project-only touched the remote cache")
	}
	code, out, stderr := invoke("profile", "check", "--config", path, "--profile", "project-only")
	if code != 0 || stderr != "" || out != "{\"profile\":\"project-only\",\"missing_includes\":[]}\n" {
		t.Fatal("local-only profile check required remote storage")
	}
}

func TestProjectOverlaysFreshFilesWithoutCachingThem(t *testing.T) {
	path, _, cfg := projectFixture(t)
	writeProjectConfig(t, path, cfg)
	if err := os.WriteFile(".env", []byte("API_KEY=base\nPROJECT_ONLY=local\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".env.local", []byte("API_KEY=override\n"), 0600); err != nil {
		t.Fatal(err)
	}
	values := projectOutput(t, path, "project")
	if values["API_KEY"] != "override" || values["PROJECT_ONLY"] != "local" || values["SERVICE_TOKEN"] != "FETCHED_SENTINEL" {
		t.Fatal("project did not overlay remote sources with local inputs")
	}
	parsed, err := config.ReadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := storage.SourceScope(parsed.Sources, parsed.SourceSequence)
	if err != nil {
		t.Fatal(err)
	}
	store, err := openProtectedStore(t.Context(), parsed.Storage, storage.Unattended)
	if err != nil {
		t.Fatal(err)
	}
	cached, found, err := store.ReadSnapshot(t.Context(), scope, time.Hour)
	if closeErr := store.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil || !found || cached.Secrets.Entries["API_KEY"] != "DISCARDED_SENTINEL" {
		t.Fatal("remote cache was not retained independently")
	}
	if _, exists := cached.Secrets.Entries["PROJECT_ONLY"]; exists {
		t.Fatal("local file value leaked into the remote cache")
	}
	t.Setenv("SI_PROVIDER_FAIL", "1")
	if err := os.WriteFile(".env.local", []byte("API_KEY=fresh\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if values := projectOutput(t, path, "project"); values["API_KEY"] != "fresh" {
		t.Fatal("cache hit prevented fresh local file reads")
	}
	if values := projectOutput(t, path, "shell"); values["API_KEY"] != "DISCARDED_SENTINEL" || values["PROJECT_ONLY"] != "" {
		t.Fatal("project values contaminated the shell profile")
	}
	if code, out, _ := invoke("--config", path, "--profile", "project", "--force"); code != 1 || out != "" {
		t.Fatal("force did not refresh remote sources")
	}
	if err := os.WriteFile(".env.local", []byte("TOKEN=\"RAW_LOCAL_SENTINEL"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := invoke("--config", path, "--profile", "project")
	if code != 1 || out != "" || strings.Contains(stderr, "RAW_LOCAL_SENTINEL") {
		t.Fatal("bad local input was ignored or leaked through diagnostics")
	}
}
