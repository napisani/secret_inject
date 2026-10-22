package envfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCurrentDirectoryOrderingAndFreshReads(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		".env":               "VALUE=base\nBASE=present\n",
		"a.env":              "VALUE=a\n",
		"z.env":              "VALUE=z\n",
		".env.local":         "VALUE=local\n",
		"z.env.local":        "VALUE=last\n",
		".env.production":    "PRODUCTION=excluded\n",
		".env.example":       "EXAMPLE=excluded\n",
		"nested/ignored.env": "NESTED=excluded\n",
		"../parent.env":      "PARENT=excluded\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(root)
	patterns := []string{"*.env", "*.env.local", ".env"}
	got, err := Load(patterns)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 2 || got.Entries["VALUE"] != "last" || got.Entries["BASE"] != "present" {
		t.Fatal("wrong file selection, ordering, or duplicate-file handling")
	}
	if err := os.WriteFile("z.env.local", []byte("VALUE=fresh\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = Load(patterns)
	if err != nil || got.Entries["VALUE"] != "fresh" {
		t.Fatal("local files were not reread")
	}
}

func TestDotenvIsDataAndErrorsAreRedacted(t *testing.T) {
	t.Chdir(t.TempDir())
	data := "export QUOTED='spaces # literal'\r\nEMPTY=\r\nMULTI=\"first\\nsecond\"\nCOMMAND='$(touch executed)'\n"
	if err := os.WriteFile(".env", []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Load([]string{"*.env"})
	if err != nil || got.Entries["QUOTED"] != "spaces # literal" || got.Entries["MULTI"] != "first\nsecond" || got.Entries["COMMAND"] != "$(touch executed)" {
		t.Fatal("dotenv parsing changed data")
	}
	if _, err := os.Stat("executed"); !os.IsNotExist(err) {
		t.Fatal("dotenv command was executed")
	}
	if err := os.WriteFile(".env", []byte("SECRET=\"RAW_PRIVATE_SENTINEL"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load([]string{"*.env"}); err == nil || strings.Contains(err.Error(), "RAW_PRIVATE_SENTINEL") {
		t.Fatal("parse error was missing or exposed file content")
	}
}

func TestPatternsAndMissingFiles(t *testing.T) {
	for _, pattern := range []string{"", "../*.env", "nested/*.env", `nested\*.env`, "[", ".", ".."} {
		if err := ValidatePatterns([]string{pattern}); err == nil {
			t.Errorf("accepted nonlocal or invalid pattern %q", pattern)
		}
	}
	t.Chdir(t.TempDir())
	got, err := Load([]string{"*.env", "*.env.local"})
	if err != nil || len(got.Entries) != 0 {
		t.Fatal("missing optional files should produce an empty collection")
	}
}

func TestSymlinkOutsideCurrentDirectoryIsRefused(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside")
	if err := os.WriteFile(outside, []byte("OUTSIDE=excluded\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".env")); err != nil {
		t.Skip("symlink creation unavailable")
	}
	t.Chdir(root)
	if _, err := Load([]string{"*.env"}); err == nil {
		t.Fatal("followed an env file outside the current directory")
	}
}
