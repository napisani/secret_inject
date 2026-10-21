package source

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"time"

	"github.com/napisani/secret_inject/internal/secret"
)

var (
	commandTimeout = 30 * time.Second
	runCLICommand  = defaultRunCLICommand
	lookupBinary   = exec.LookPath
)

func defaultRunCLICommand(name string, env []string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	output, err := cmd.Output()
	if err != nil {
		// exec.ExitError retains raw provider stderr, which may contain secrets.
		// Only classified metadata may enter the returned error chain.
		provider, operation := commandOperation(name, args)
		failure := &ProviderError{Provider: provider, Operation: operation, Kind: "start", ExitCode: -1}
		var exit *exec.ExitError
		if ctx.Err() != nil {
			failure.Kind = "timeout"
		} else if errors.As(err, &exit) {
			failure.Kind = "exit"
			failure.ExitCode = exit.ExitCode()
		}
		return nil, failure
	}
	return output, nil
}

type ProviderError struct {
	Provider  string
	Operation string
	Kind      string
	ExitCode  int
}

func (e *ProviderError) Error() string {
	if e.Kind == "exit" {
		return fmt.Sprintf(
			"provider=%s operation=%s failure=%s exit=%d",
			e.Provider,
			e.Operation,
			e.Kind,
			e.ExitCode,
		)
	}
	return fmt.Sprintf(
		"provider=%s operation=%s failure=%s",
		e.Provider,
		e.Operation,
		e.Kind,
	)
}

func commandOperation(name string, args []string) (string, string) {
	switch name {
	case "doppler":
		return "doppler", "fetch"
	case "op":
		return "onepassword", "read"
	case "bws":
		if len(args) > 1 && args[1] == "get" {
			return "bitwarden", "get"
		}
		return "bitwarden", "list"
	default:
		return "provider", "fetch"
	}
}

func Name(src Source) string {
	switch src.(type) {
	case *Doppler:
		return "doppler"
	case *OnePassword:
		return "onepassword"
	case *Bitwarden:
		return "bitwarden"
	default:
		return "provider"
	}
}

func buildCommandEnv(previous *secret.Secrets) []string {
	env := os.Environ()
	if previous == nil || len(previous.Entries) == 0 {
		return env
	}

	keys := make([]string, 0, len(previous.Entries))
	for key := range previous.Entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		env = append(env, fmt.Sprintf("%s=%s", key, previous.Entries[key]))
	}
	return env
}
