package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"strings"

	"github.com/napisani/secret_inject/internal/config"
	"github.com/napisani/secret_inject/internal/source"
	"github.com/napisani/secret_inject/internal/storage"
	"golang.org/x/term"
)

type enrollmentInput interface {
	Available() bool
	ReadToken() (string, error)
}
type stdinEnrollment struct{}

var enrollmentTerminal enrollmentInput = stdinEnrollment{}

func (stdinEnrollment) Available() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
func (stdinEnrollment) ReadToken() (string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return "", errors.New("enrollment terminal unavailable")
	}
	if _, err := tty.Write([]byte("Credential token (hidden): ")); err != nil {
		return "", errors.Join(errors.New("enrollment terminal output failed"), safeTerminalClose(tty))
	}
	value, readErr := term.ReadPassword(int(os.Stdin.Fd()))
	_, writeErr := tty.Write([]byte("\n"))
	closeErr := safeTerminalClose(tty)
	if readErr != nil || writeErr != nil || closeErr != nil {
		return "", errors.New("enrollment input failed")
	}
	return string(value), nil
}
func safeTerminalClose(file *os.File) error {
	if err := file.Close(); err != nil {
		return errors.New("enrollment terminal finalization failed")
	}
	return nil
}

type credentialReceipt struct {
	SchemaVersion int                        `json:"schema_version"`
	Credential    storage.CredentialMetadata `json:"credential"`
	Operation     string                     `json:"operation"`
}

func credentialCommand(argv []string, stdout, stderr io.Writer) int {
	if len(argv) == 0 {
		return reportCommandFailure(stderr, failureUsage)
	}
	operation := argv[0]
	if operation != "list" && operation != "set" && operation != "remove" {
		return reportCommandFailure(stderr, failureUsage)
	}
	var name string
	flags := argv[1:]
	if operation != "list" && len(flags) > 0 && !strings.HasPrefix(flags[0], "-") {
		name = flags[0]
		flags = flags[1:]
	}
	var parseOutput strings.Builder
	fs := flag.NewFlagSet("secret_inject credential "+operation, flag.ContinueOnError)
	fs.SetOutput(&parseOutput)
	filename := fs.String("config", defaultFile, "Config file path")
	var provider string
	if operation == "set" {
		fs.StringVar(&provider, "provider", "", "Credential provider kind")
	}
	err := fs.Parse(flags)
	if errors.Is(err, flag.ErrHelp) {
		if err := writeOutput(stderr, []byte(parseOutput.String())); err != nil {
			return 1
		}
		return 0
	}
	if err != nil || fs.NArg() != 0 || operation != "list" && !source.ValidCredentialName(name) || operation == "set" && !source.KnownProvider(provider) {
		return reportCommandFailure(stderr, failureUsage)
	}
	if operation == "set" && !enrollmentTerminal.Available() {
		return reportCommandFailure(stderr, failureUsage)
	}
	cfg, err := config.ReadConfig(*filename)
	if err != nil {
		return reportCommandFailure(stderr, failureConfiguration)
	}
	if err := storage.ValidateNativeConfig(cfg.Storage); err != nil {
		return reportCommandFailure(stderr, failureStoreUnavailable)
	}
	policy := storage.Unattended
	if operation == "set" {
		policy = storage.InteractiveSetup
	}
	ctx := context.Background()
	var stop func()
	if operation != "set" {
		ctx, stop = commandContext()
		defer stop()
	}
	store, err := openProtectedStore(ctx, cfg.Storage, policy)
	if err != nil {
		return reportCommandFailure(stderr, preparationFailure(ctx, err))
	}
	fail := func(err error) int {
		closeErr := store.Close()
		if closeErr != nil {
			return reportCommandFailure(stderr, failureFinalization)
		}
		return reportCommandFailure(stderr, preparationFailure(ctx, err))
	}
	var response interface{}
	switch operation {
	case "list":
		metadata, err := store.ListCredentials(ctx)
		if err != nil {
			return fail(err)
		}
		response = struct {
			SchemaVersion int                          `json:"schema_version"`
			Credentials   []storage.CredentialMetadata `json:"credentials"`
		}{1, metadata}
	case "set":
		metadata, err := store.ListCredentials(ctx)
		if err != nil {
			return fail(err)
		}
		for _, existing := range metadata {
			if existing.Name == name && existing.Provider != provider {
				return fail(storage.ErrCredentialProviderMismatch)
			}
		}
		value, err := enrollmentTerminal.ReadToken()
		if err != nil {
			return fail(errors.New("enrollment input failed"))
		}
		if !source.ValidToken(value) {
			if closeErr := store.Close(); closeErr != nil {
				return reportCommandFailure(stderr, failureFinalization)
			}
			return reportCommandFailure(stderr, failureUsage)
		}
		ctx, stop = commandContext()
		defer stop()
		if err := store.SetCredential(ctx, name, provider, value); err != nil {
			return fail(err)
		}
		response = credentialReceipt{1, storage.CredentialMetadata{Name: name, Provider: provider}, "set"}
	case "remove":
		removed, err := store.RemoveCredential(ctx, name)
		if err != nil {
			return fail(err)
		}
		response = credentialReceipt{1, removed, "remove"}
	}
	if err := store.Close(); err != nil {
		return reportCommandFailure(stderr, failureFinalization)
	}
	if err := ctx.Err(); err != nil {
		return reportCommandFailure(stderr, preparationFailure(ctx, err))
	}
	if err := writeJSON(stdout, response); err != nil {
		return reportCommandFailure(stderr, failureMetadataOutput)
	}
	return 0
}

func storeCommand(argv []string, stdout, stderr io.Writer) int {
	if len(argv) == 0 || argv[0] != "prepare" {
		return reportCommandFailure(stderr, failureUsage)
	}
	var parseOutput strings.Builder
	fs := flag.NewFlagSet("secret_inject store prepare", flag.ContinueOnError)
	fs.SetOutput(&parseOutput)
	filename := fs.String("config", defaultFile, "Config file path")
	err := fs.Parse(argv[1:])
	if errors.Is(err, flag.ErrHelp) {
		if err := writeOutput(stderr, []byte(parseOutput.String())); err != nil {
			return 1
		}
		return 0
	}
	if err != nil || fs.NArg() != 0 {
		return reportCommandFailure(stderr, failureUsage)
	}
	cfg, err := config.ReadConfig(*filename)
	if err != nil {
		return reportCommandFailure(stderr, failureConfiguration)
	}
	if err := storage.ValidateNativeConfig(cfg.Storage); err != nil {
		return reportCommandFailure(stderr, failureStoreUnavailable)
	}
	store, err := openProtectedStore(context.Background(), cfg.Storage, storage.InteractiveSetup)
	if err != nil {
		return reportCommandFailure(stderr, preparationFailure(context.Background(), err))
	}
	accessErr := store.PrepareAccess(context.Background())
	if err := store.Close(); err != nil {
		return reportCommandFailure(stderr, failureFinalization)
	}
	if accessErr != nil {
		return reportCommandFailure(stderr, preparationFailure(context.Background(), accessErr))
	}
	response := struct {
		SchemaVersion int `json:"schema_version"`
		Store         struct {
			Kind string `json:"kind"`
		} `json:"store"`
	}{SchemaVersion: 1}
	response.Store.Kind = "prepared"
	if err := writeJSON(stdout, response); err != nil {
		return reportCommandFailure(stderr, failureMetadataOutput)
	}
	return 0
}
