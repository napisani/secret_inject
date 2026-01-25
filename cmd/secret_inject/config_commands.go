package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const configTemplate = `{
  "source_sequence": ["doppler", "onepassword", "bitwarden"],
  "sources": {
    "doppler": {
      "project": "my-project",
      "env": "dev"
    },
    "onepassword": {
      "secrets": {
        "API_TOKEN": "op://Production/API/token",
        "DB_USER": "op://Production/Database/username"
      }
    },
    "bitwarden": {
      "secrets": {
        "DB_PASSWORD": "382580ab-1368-4e85-bfa3-b02e01400c9f",
        "API_TOKEN": {
          "key": "api-token",
          "project_id": "e325ea69-a3ab-4dff-836f-b02e013fe530"
        }
      }
    }
  },
  "storage": {
    "type": "keyring",
    "allowed_backends": ["keychain", "secret-service", "wincred"]
  }
}
`

func runConfigCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: secret_inject config <init|edit> [flags]")
	}

	switch args[0] {
	case "init":
		fs := flag.NewFlagSet("config init", flag.ContinueOnError)
		discard := strings.Builder{}
		fs.SetOutput(&discard)
		configPath := fs.String("config", defaultFile, "Config file path")
		force := fs.Bool("force", false, "Overwrite existing config file")
		if err := fs.Parse(args[1:]); err != nil {
			if err == flag.ErrHelp {
				return nil
			}
			return err
		}
		if err := initConfigFile(*configPath, *force); err != nil {
			return err
		}
		fmt.Printf("Config written to %s\n", *configPath)
		return nil
	case "edit":
		fs := flag.NewFlagSet("config edit", flag.ContinueOnError)
		discard := strings.Builder{}
		fs.SetOutput(&discard)
		configPath := fs.String("config", defaultFile, "Config file path")
		editorFlag := fs.String("editor", "", "Editor override (defaults to $EDITOR or vi)")
		if err := fs.Parse(args[1:]); err != nil {
			if err == flag.ErrHelp {
				return nil
			}
			return err
		}
		return editConfigFile(*configPath, *editorFlag)
	default:
		return fmt.Errorf("unknown config subcommand %q", args[0])
	}
}

func initConfigFile(path string, force bool) error {
	if path == "" {
		return errors.New("config path cannot be empty")
	}

	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("config file %s already exists (use --force to overwrite)", path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(configTemplate), 0o600); err != nil {
		return fmt.Errorf("writing config template: %w", err)
	}

	return nil
}

func editConfigFile(path string, overrideEditor string) error {
	if path == "" {
		return errors.New("config path cannot be empty")
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("config file %s does not exist (run 'secret_inject config init' first)", path)
	} else if err != nil {
		return err
	}

	editor := strings.TrimSpace(overrideEditor)
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		editor = "vi"
	}

	parts, err := splitCommandLine(editor)
	if err != nil {
		return fmt.Errorf("parsing editor command: %w", err)
	}
	if len(parts) == 0 {
		return errors.New("editor command is empty")
	}

	return editorLauncher(parts[0], parts[1:], path)
}

func splitCommandLine(input string) ([]string, error) {
	var args []string
	var current strings.Builder
	var quote rune
	escaped := false

	flush := func() {
		if current.Len() > 0 {
			args = append(args, current.String())
			current.Reset()
		}
	}

	for _, r := range input {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}

		switch {
		case r == '\\':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t':
			flush()
		default:
			current.WriteRune(r)
		}
	}

	if escaped {
		return nil, errors.New("unfinished escape sequence in command")
	}

	if quote != 0 {
		return nil, errors.New("unterminated quote in command")
	}

	flush()
	return args, nil
}

var editorLauncher = defaultEditorLauncher

func defaultEditorLauncher(command string, args []string, path string) error {
	cmd := exec.Command(command, append(args, path)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func readConfigTemplate() string {
	return configTemplate
}
