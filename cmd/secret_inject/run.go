package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/napisani/secret_inject/internal/config"
	"github.com/napisani/secret_inject/internal/diagnostics"
	"github.com/napisani/secret_inject/internal/secret"
)

var replaceProcess = platformExec

func runCommand(argv []string, stdout, stderr io.Writer) int {
	separator := -1
	for index, value := range argv {
		if value == "--" {
			separator = index
			break
		}
	}
	var parseOutput strings.Builder
	fs := flag.NewFlagSet("secret_inject run", flag.ContinueOnError)
	fs.SetOutput(&parseOutput)
	args := Args{TTL: time.Hour}
	fs.StringVar(&args.ConfigFile, "config", defaultFile, "Config file path")
	fs.StringVar(&args.Profile, "profile", "", "Required named profile")
	fs.BoolVar(&args.Debug, "debug", false, "Write private rotating debug log")
	fs.BoolVar(&args.Force, "force", false, "Force fresh remote snapshot")
	fs.DurationVar(&args.TTL, "ttl", time.Hour, "Cache TTL")
	flags := argv
	if separator >= 0 {
		flags = argv[:separator]
	}
	parseErr := fs.Parse(flags)
	if errors.Is(parseErr, flag.ErrHelp) {
		if err := writeOutput(stderr, []byte(parseOutput.String())); err != nil {
			return 1
		}
		return 0
	}
	if parseErr != nil || fs.NArg() != 0 || separator < 0 || separator+1 >= len(argv) || args.Profile == "" {
		return reportCommandFailure(stderr, failureUsage)
	}
	args.ProfileSet = true
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return reportCommandFailure(stderr, failureUnsupportedPlatform)
	}
	cfg, err := config.ReadConfig(args.ConfigFile)
	if err != nil {
		return reportCommandFailure(stderr, failureConfiguration)
	}
	selected, err := cfg.Profiles.Choose(&args.Profile)
	if err != nil {
		return reportCommandFailure(stderr, failureProfileSelection)
	}
	targetArgv := append([]string{}, argv[separator+1:]...)
	binary, err := exec.LookPath(targetArgv[0])
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return reportCommandFailure(stderr, failureExecutableMissing)
		}
		return reportCommandFailure(stderr, failureExecutablePermission)
	}
	ctx, stop := commandContext()
	defer stop()
	var debug *diagnostics.Session
	if args.Debug {
		debug, err = diagnostics.Open()
		if err != nil {
			return reportCommandFailure(stderr, failureFinalization)
		}
		debug.Record(diagnostics.Event{Kind: diagnostics.ProfileSelected, Profile: args.Profile})
	}
	var loaded *resolvedSnapshot
	abort := func(failure commandFailure) int {
		if loaded != nil {
			if err := loaded.Close(); err != nil {
				failure = failureFinalization
			}
		}
		if err := debug.Finish(); err != nil {
			failure = failureFinalization
		}
		return reportCommandFailure(stderr, failure)
	}
	loaded, err = loadSnapshot(ctx, cfg, args, selected, debug)
	if err != nil {
		return abort(preparationFailure(ctx, err))
	}
	projected, events, err := selected.Project(loaded.Values)
	debug.RecordSelection(events)
	if err != nil {
		return abort(failureProjection)
	}
	env, err := targetEnvironment(os.Environ(), projected)
	if err != nil {
		return abort(failureEnvironment)
	}
	if err := loaded.Finish(ctx); err != nil {
		return abort(preparationFailure(ctx, err))
	}
	debug.Record(diagnostics.Event{Kind: diagnostics.ResultPrepared, Operation: "run", Count: len(projected.Entries)})
	if err := debug.Finish(); err != nil {
		return reportCommandFailure(stderr, failureFinalization)
	}
	if err := ctx.Err(); err != nil {
		return reportCommandFailure(stderr, preparationFailure(ctx, err))
	}
	stop()
	if err := replaceProcess(binary, targetArgv, env); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return reportCommandFailure(stderr, failureExecutableMissing)
		}
		return reportCommandFailure(stderr, failureExecutablePermission)
	}
	return 0
}

func targetEnvironment(parent []string, selected *secret.Secrets) ([]string, error) {
	values := make(map[string]string, len(parent)+len(selected.Entries))
	for _, entry := range parent {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" || strings.ContainsRune(entry, '\x00') {
			return nil, errors.New("invalid inherited environment")
		}
		values[key] = value
	}
	for key, value := range selected.Entries {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, '\x00') {
			return nil, errors.New("invalid selected environment")
		}
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env, nil
}
