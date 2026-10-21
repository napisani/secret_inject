package output

import (
	"encoding/json"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/napisani/secret_inject/internal/secret"
)

func TestRenderShellPreservesConsumingShellState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell required")
	}
	s := secret.New()
	s.Entries = map[string]string{"SET": "fetched", "EMPTY": "fetched", "LOCAL": "fetched", "NEW": "a'\n$HOME`false`"}
	rendered, err := Render(s, "shell", false)
	if err != nil {
		t.Fatal(err)
	}
	script := "SET=old; EMPTY=''; LOCAL=local; export SET EMPTY;\n" + string(rendered) + "\nprintf '%s\\n' \"$SET\" \"$EMPTY\" \"$LOCAL\" \"$NEW\"; env | grep '^LOCAL=' || :"
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old\n\nlocal\na'\n$HOME`false`\n" {
		t.Fatalf("consuming-shell behavior: %q", got)
	}
	rendered, err = Render(s, "shell", true)
	if err != nil || strings.Contains(string(rendered), "if [") {
		t.Fatal("overwrite exports must be unconditional")
	}
}

func TestRenderFormatsAndValidation(t *testing.T) {
	s := secret.New()
	s.Entries = map[string]string{"Z": "line\nquote\"$", "A": ""}
	b, err := Render(s, "json", false)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(b, &decoded); err != nil || decoded["Z"] != s.Entries["Z"] {
		t.Fatal("JSON changed values")
	}
	b, err = Render(s, "env", false)
	if err != nil || !strings.HasPrefix(string(b), "A=\"\"\nZ=") {
		t.Fatal("env not sorted")
	}
	if b, err := Render(s, "unsupported", true); err == nil || len(b) > 0 {
		t.Fatal("unknown format emitted secrets")
	}
	s.Entries["X;false"] = "value"
	for _, format := range []string{"shell", "json", "env"} {
		if b, err := Render(s, format, true); err == nil || len(b) > 0 {
			t.Fatal("invalid name emitted output")
		}
	}
}
