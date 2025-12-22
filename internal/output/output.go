package output

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/napisani/secret_inject/internal/secret"
)

func ExportShell(secrets *secret.Secrets) {
	slog.Debug("Exporting secrets as shell commands")
	var str strings.Builder

	for key, value := range secrets.Entries {
		escapedValue := escapeShellValue(value)
		str.WriteString(fmt.Sprintf("export %s='%s'\n", key, escapedValue))
	}
	fmt.Print(str.String())
}

// escapeShellValue escapes single quotes for use in single-quoted shell strings
// Uses the POSIX pattern: replace ' with '\”
func escapeShellValue(value string) string {
	// In a single-quoted string, to include a literal single quote:
	// End the quote, add an escaped quote, restart the quote
	// Example: 'it's' becomes 'it'\''s'
	return strings.ReplaceAll(value, "'", "'\\''")
}

func ExportJSON(secrets *secret.Secrets) {
	slog.Debug("Exporting secrets as JSON")
	output := make(map[string]string)
	for key, value := range secrets.Entries {
		output[key] = value
	}

	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		slog.Error("Error marshaling JSON", "error", err)
		return
	}
	fmt.Println(string(jsonBytes))
}

func ExportEnv(secrets *secret.Secrets) {
	slog.Debug("Exporting secrets as env file format")
	var str strings.Builder

	for key, value := range secrets.Entries {
		escapedValue := escapeEnvValue(value)
		str.WriteString(fmt.Sprintf("%s=\"%s\"\n", key, escapedValue))
	}
	fmt.Print(str.String())
}

// escapeEnvValue escapes special characters for .env file format
func escapeEnvValue(value string) string {
	// Escape in order: backslash first (to avoid double-escaping), then others
	value = strings.ReplaceAll(value, "\\", "\\\\") // \ -> \\
	value = strings.ReplaceAll(value, "\"", "\\\"") // " -> \"
	value = strings.ReplaceAll(value, "\n", "\\n")  // newline -> \n
	value = strings.ReplaceAll(value, "\r", "\\r")  // carriage return -> \r
	value = strings.ReplaceAll(value, "\t", "\\t")  // tab -> \t
	value = strings.ReplaceAll(value, "$", "\\$")   // $ -> \$ (prevent variable expansion)
	return value
}

func Export(secrets *secret.Secrets, format string) {
	switch format {
	case "shell":
		ExportShell(secrets)
	case "json":
		ExportJSON(secrets)
	case "env":
		ExportEnv(secrets)
	default:
		slog.Error("Unknown output format", "format", format)
		ExportShell(secrets)
	}
}
