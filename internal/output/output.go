package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/napisani/secret_inject/internal/profile"
	"github.com/napisani/secret_inject/internal/secret"
)

func ValidFormat(format string) bool { return format == "shell" || format == "json" || format == "env" }

func Render(secrets *secret.Secrets, format string, overwrite bool) ([]byte, error) {
	if !ValidFormat(format) {
		return nil, errors.New("output must be shell, json, or env")
	}
	keys := make([]string, 0, len(secrets.Entries))
	for key := range secrets.Entries {
		if !profile.ValidName(key) {
			return nil, errors.New("invalid output variable name")
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if format == "json" {
		b, err := json.MarshalIndent(secrets.Entries, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(b, '\n'), nil
	}
	var b strings.Builder
	for _, key := range keys {
		value := secrets.Entries[key]
		if format == "env" {
			fmt.Fprintf(
				&b,
				"%s=\"%s\"\n",
				key,
				escapeEnvValue(value),
			)
			continue
		}
		if !overwrite {
			fmt.Fprintf(&b, "if [ \"${%s+x}\" != x ]; then\n  ", key)
		}
		fmt.Fprintf(
			&b,
			"export %s='%s'\n",
			key,
			escapeShellValue(value),
		)
		if !overwrite {
			b.WriteString("fi\n")
		}
	}
	return []byte(b.String()), nil
}

func escapeShellValue(value string) string { return strings.ReplaceAll(value, "'", "'\\''") }

func escapeEnvValue(value string) string {
	// Backslashes must be escaped before introducing escapes for other characters.
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\r", "\\r")
	value = strings.ReplaceAll(value, "\t", "\\t")
	return strings.ReplaceAll(value, "$", "\\$")
}
