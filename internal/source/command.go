package source

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
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
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
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
