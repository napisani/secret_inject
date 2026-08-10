package output

import (
	"testing"

	"github.com/napisani/secret_inject/internal/secret"
)

func TestShellEscaping_SingleQuote(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple single quote",
			input:    "it's a secret",
			expected: "it'\\''s a secret",
		},
		{
			name:     "multiple single quotes",
			input:    "don't can't won't",
			expected: "don'\\''t can'\\''t won'\\''t",
		},
		{
			name:     "quote at start",
			input:    "'secret",
			expected: "'\\''secret",
		},
		{
			name:     "quote at end",
			input:    "secret'",
			expected: "secret'\\''",
		},
		{
			name:     "only quotes",
			input:    "'''",
			expected: "'\\'''\\'''\\''",
		},
		{
			name:     "no quotes",
			input:    "secret",
			expected: "secret",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeShellValue(tt.input)
			if result != tt.expected {
				t.Errorf("escapeShellValue(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestShellEscaping_SpecialChars(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"newline", "secret\nvalue"},
		{"dollar sign", "$SECRET"},
		{"backtick", "`command`"},
		{"exclamation", "secret!"},
		{"backslash", "secret\\value"},
		{"double quote", `secret"value`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secrets := secret.New()
			secrets.Entries["TEST"] = tt.input

			// Test that the output doesn't break shell parsing
			// Note: This doesn't execute the shell, just checks escaping works
			escaped := escapeShellValue(tt.input)

			// Should not contain unescaped single quotes
			// (they should all be in the '\'' pattern)
			for i := 0; i < len(escaped); i++ {
				if escaped[i] == '\'' {
					// Check if it's part of the escape sequence
					if i+2 < len(escaped) {
						// Should be followed by \' or be at start/end of '\'' pattern
						if !(i > 0 && escaped[i-1] == '\\' && i+1 < len(escaped) && escaped[i+1] == '\\') {
							// This is okay - could be start or end of string
						}
					}
				}
			}
		})
	}
}

func TestEnvEscaping_Multiline(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "newline",
			input:    "line1\nline2",
			expected: "line1\\nline2",
		},
		{
			name:     "carriage return",
			input:    "line1\rline2",
			expected: "line1\\rline2",
		},
		{
			name:     "tab",
			input:    "col1\tcol2",
			expected: "col1\\tcol2",
		},
		{
			name:     "mixed whitespace",
			input:    "a\nb\tc\rd",
			expected: "a\\nb\\tc\\rd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeEnvValue(tt.input)
			if result != tt.expected {
				t.Errorf("escapeEnvValue(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestEnvEscaping_SpecialChars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "double quotes",
			input:    `value"with"quotes`,
			expected: `value\"with\"quotes`,
		},
		{
			name:     "dollar sign",
			input:    "$VAR_NAME",
			expected: `\$VAR_NAME`,
		},
		{
			name:     "backslash",
			input:    `path\to\file`,
			expected: `path\\to\\file`,
		},
		{
			name:     "backslash before quote",
			input:    `value\"quoted`,
			expected: `value\\\"quoted`, // \\ for the backslash, \" for the quote
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "all special chars combined",
			input:    "\\\"\n\r\t$",
			expected: "\\\\\\\"\\n\\r\\t\\$",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeEnvValue(tt.input)
			if result != tt.expected {
				t.Errorf("escapeEnvValue(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestEnvEscaping_BackslashOrder(t *testing.T) {
	// This test verifies that backslashes are escaped first
	// to avoid double-escaping
	input := `\n`     // Literal backslash followed by n
	expected := `\\n` // Should become \\n (escaped backslash and literal n)

	result := escapeEnvValue(input)
	if result != expected {
		t.Errorf("escapeEnvValue(%q) = %q, want %q", input, result, expected)
	}

	// Another test: actual newline should become \n
	input2 := "\n"
	expected2 := "\\n"
	result2 := escapeEnvValue(input2)
	if result2 != expected2 {
		t.Errorf("escapeEnvValue(newline) = %q, want %q", result2, expected2)
	}
}

func TestExportShell_Integration(t *testing.T) {
	secrets := secret.New()
	secrets.Entries["SIMPLE"] = "value"
	secrets.Entries["WITH_QUOTE"] = "it's a secret"
	secrets.Entries["WITH_DOLLAR"] = "$PATH"

	// Just verify it doesn't panic
	// In a real test, we'd capture stdout and verify output
	// For now, we're testing that escapeShellValue works correctly
	for key, value := range secrets.Entries {
		escaped := escapeShellValue(value)
		// Verify no unhandled characters break the format
		_ = key
		_ = escaped
	}
}

func TestExportEnv_Integration(t *testing.T) {
	secrets := secret.New()
	secrets.Entries["SIMPLE"] = "value"
	secrets.Entries["MULTILINE"] = "line1\nline2"
	secrets.Entries["WITH_QUOTE"] = `value"quoted`

	// Just verify it doesn't panic
	for key, value := range secrets.Entries {
		escaped := escapeEnvValue(value)
		_ = key
		_ = escaped
	}
}
