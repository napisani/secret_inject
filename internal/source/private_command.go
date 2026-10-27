package source

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/napisani/secret_inject/internal/privatefs"
)

type providerCommand func(string, []string, ...string) ([]byte, error)

func sourceCommand(command providerCommand) providerCommand {
	if command != nil {
		return command
	}
	return runCLICommand
}

// Entries require the native/provider conformance evidence described in the spec.
// Source-audited candidates alone cannot authorize an unattended fetch recipe.
var certifiedProviderReleases = map[string]map[string]bool{}

func certifiedProviderReady(provider string) bool {
	for version, verified := range certifiedProviderReleases[provider] {
		if verified && version != "" && version == strings.TrimSpace(version) && !strings.ContainsAny(version, "\r\n\x00") {
			return true
		}
	}
	return false
}

type providerContext struct {
	directory   string
	binary      string
	provider    string
	environment []string
	prefix      []string
	tokenName   string
}

func fetchPrivateProvider(ctx context.Context, provider string, raw json.RawMessage, token string) (values map[string]string, err error) {
	factory, known := sourceRegistry[provider]
	if !known {
		return nil, ErrCredentialBinding
	}
	src := factory()
	if err := src.Init(raw); err != nil {
		return nil, &ProviderError{Provider: provider, Operation: "initialize", Kind: "configuration", ExitCode: -1}
	}
	if !certifiedProviderReady(provider) {
		return nil, ErrProviderVersion
	}
	private, err := newProviderContext(provider)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cleanupErr := os.RemoveAll(private.directory); cleanupErr != nil {
			values = nil
			err = errors.Join(err, ErrProviderCleanup)
		}
	}()
	version, err := private.execute(ctx, "", "--version")
	if err != nil {
		return nil, err
	}
	versionText := strings.TrimSpace(string(version))
	if versionText == "" || strings.ContainsAny(versionText, "\r\n\x00") || !certifiedProviderReleases[provider][versionText] {
		return nil, ErrProviderVersion
	}
	run := providerCommand(func(_ string, _ []string, args ...string) ([]byte, error) {
		return private.execute(ctx, token, args...)
	})
	switch src := src.(type) {
	case *Doppler:
		src.command = run
	case *OnePassword:
		src.command = run
	case *Bitwarden:
		src.command = run
	default:
		return nil, ErrCredentialBinding
	}
	rawValues, err := src.GetAllSecrets(nil)
	if err != nil {
		return nil, err
	}
	return rawValues.Entries, nil
}

func newProviderContext(provider string) (_ *providerContext, err error) {
	binary := map[string]string{"doppler": "doppler", "onepassword": "op", "bitwarden": "bws"}[provider]
	if binary == "" {
		return nil, ErrCredentialBinding
	}
	resolved, err := exec.LookPath(binary)
	if err != nil {
		return nil, &ProviderError{Provider: provider, Operation: "initialize", Kind: "executable", ExitCode: -1}
	}
	directory, err := os.MkdirTemp("", "secret-inject-provider-")
	if err != nil {
		return nil, errors.New("private provider context unavailable")
	}
	success := false
	defer func() {
		if !success {
			if e := os.RemoveAll(directory); e != nil {
				err = errors.Join(err, ErrProviderCleanup)
			}
		}
	}()
	if err := privatefs.EnsureDir(directory); err != nil {
		return nil, errors.New("private provider context unavailable")
	}
	p := &providerContext{directory: directory, binary: resolved, provider: provider}
	for _, entry := range os.Environ() {
		name, _, found := strings.Cut(entry, "=")
		name = strings.ToUpper(name)
		if !found || strings.HasPrefix(name, "OP_") || strings.HasPrefix(name, "BWS_") || name == "DOPPLER_TOKEN" || name == "DOPPLER_CONFIG_DIR" || strings.HasPrefix(name, "ENCLAVE_") {
			continue
		}
		p.environment = append(p.environment, entry)
	}
	switch provider {
	case "doppler":
		if err := privatefs.WriteAtomic(directory, ".doppler.yaml", []byte("{}\n")); err != nil {
			return nil, errors.New("private provider configuration failed")
		}
		p.prefix = []string{"--no-check-version", "--config-dir", directory}
		p.tokenName = "DOPPLER_TOKEN"
	case "onepassword":
		p.prefix = []string{"--config", directory, "--cache=false"}
		p.environment = append(p.environment, "OP_BIOMETRIC_UNLOCK_ENABLED=false", "OP_CACHE=false")
		p.tokenName = "OP_SERVICE_ACCOUNT_TOKEN"
	case "bitwarden":
		contents := "[profiles.default]\nstate_opt_out = \"true\"\n"
		if endpoint := os.Getenv("BWS_SERVER_URL"); endpoint != "" {
			parsed, e := url.Parse(endpoint)
			if e != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || strings.ContainsAny(endpoint, "\r\n\x00") {
				return nil, errors.New("provider endpoint configuration invalid")
			}
			contents += "server_base = " + strconv.Quote(parsed.String()) + "\n"
		}
		if err := privatefs.WriteAtomic(directory, "config", []byte(contents)); err != nil {
			return nil, errors.New("private provider configuration failed")
		}
		p.environment = append(p.environment, "BWS_CONFIG_FILE="+filepath.Join(directory, "config"), "BWS_PROFILE=default")
		p.tokenName = "BWS_ACCESS_TOKEN"
	}
	success = true
	return p, nil
}

func (p *providerContext) execute(parent context.Context, token string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, commandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, p.binary, append(append([]string{}, p.prefix...), args...)...)
	command.Env = append([]string{}, p.environment...)
	if token != "" {
		command.Env = append(command.Env, p.tokenName+"="+token)
	}
	command.Stdin = nil
	command.Stderr = io.Discard
	output, err := command.Output()
	if err != nil {
		if parent.Err() != nil {
			return nil, parent.Err()
		}
		failure := &ProviderError{Provider: p.provider, Operation: "fetch", Kind: "start", ExitCode: -1}
		if token == "" {
			failure.Operation = "version"
		}
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
