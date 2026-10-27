package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const configTemplate = `{
  "source_sequence": ["doppler", "onepassword", "bitwarden"],
  "sources": {
    "doppler": {
      "project": "my-project",
      "env": "dev",
      "credential": {"kind":"stored", "name":"doppler-root"}
    },
    "onepassword": {
      "credential": {"kind":"stored", "name":"onepassword-root"},
      "secrets": {
        "API_TOKEN": "op://Production/API/token",
        "DB_USER": "op://Production/Database/username"
      }
    },
    "bitwarden": {
      "credential": {"kind":"stored", "name":"bitwarden-root"},
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
    "allowed_backends": ["NATIVE_BACKEND"]
  }
}
`

func runConfigCommand(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: secret_inject config <init|edit> [flags]")
	}

	switch args[0] {
	case "init":
		fs := flag.NewFlagSet("config init", flag.ContinueOnError)
		var help strings.Builder
		fs.SetOutput(&help)
		configPath := fs.String("config", defaultFile, "Config file path")
		force := fs.Bool("force", false, "Overwrite existing config file")
		if err := fs.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return writeOutput(stderr, []byte(help.String()))
			}
			return err
		}
		if fs.NArg() != 0 {
			return errors.New("config init does not accept positional arguments")
		}
		if err := initConfigFile(*configPath, *force); err != nil {
			return err
		}
		message := fmt.Sprintf("Config written to %s\n", *configPath)
		return writeOutput(stdout, []byte(message))
	case "edit":
		fs := flag.NewFlagSet("config edit", flag.ContinueOnError)
		var help strings.Builder
		fs.SetOutput(&help)
		configPath := fs.String("config", defaultFile, "Config file path")
		editorFlag := fs.String("editor", "", "Editor override (defaults to $EDITOR or vi)")
		if err := fs.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return writeOutput(stderr, []byte(help.String()))
			}
			return err
		}
		if fs.NArg() != 0 {
			return errors.New("config edit does not accept positional arguments")
		}
		return editConfigFile(
			*configPath,
			*editorFlag,
			stdout,
			stderr,
		)
	default:
		return fmt.Errorf("unknown config subcommand %q", args[0])
	}
}

func initConfigFile(path string, force bool) (err error) {
	if path == "" {
		return errors.New("config path cannot be empty")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	flags := os.O_CREATE | os.O_WRONLY
	if !force {
		flags |= os.O_EXCL
	}
	file, err := os.OpenFile(path, flags, 0o600)
	if errors.Is(err, os.ErrExist) && !force {
		return fmt.Errorf("config file %s already exists (use --force to overwrite)", path)
	}
	if err != nil {
		return fmt.Errorf("opening config template: %w", err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("restricting config permissions: %w", err)
	}
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("truncating config template: %w", err)
	}
	if _, err := file.Write([]byte(readConfigTemplate())); err != nil {
		return fmt.Errorf("writing config template: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("syncing config template: %w", err)
	}
	return nil
}

func editConfigFile(path, overrideEditor string, stdout, stderr io.Writer) error {
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

	return editorLauncher(
		parts[0],
		append(parts[1:], path),
		stdout,
		stderr,
	)
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

func defaultEditorLauncher(command string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.Command(command, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func readConfigTemplate() string {
	backend := map[string]string{"darwin": "darwin-keychain", "linux": "linux-secret-service"}[runtime.GOOS]
	return strings.ReplaceAll(configTemplate, "NATIVE_BACKEND", backend)
}
