package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/napisani/secret_inject/internal/config"
	"github.com/napisani/secret_inject/internal/diagnostics"
	"github.com/napisani/secret_inject/internal/output"
)

type Args struct {
	ConfigFile string
	Clean      bool
	Debug      bool
	Force      bool
	TTL        time.Duration
	Output     string
	Version    bool
	Profile    string
	ProfileSet bool
	Check      bool
}

var defaultFile = path.Join(os.Getenv("HOME"), ".config", ".secret_inject.json")

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func parseArgs(argv []string, stderr io.Writer) (Args, error) {
	var args Args
	if len(argv) > 0 && argv[0] == "profile" {
		if len(argv) < 2 || argv[1] != "check" {
			return args, errors.New("usage: secret_inject profile check --profile NAME [flags]")
		}
		args.Check = true
		argv = argv[2:]
	}
	commandName := "secret_inject"
	if args.Check {
		commandName += " profile check"
	}
	fs := flag.NewFlagSet(commandName, flag.ContinueOnError)
	var parseOutput strings.Builder
	fs.SetOutput(&parseOutput)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage of "+commandName+":")
		fs.PrintDefaults()
		if !args.Check {
			fmt.Fprintln(fs.Output(), "\nCommands: run, profile list, profile describe, profile check, credential set/list/remove, store prepare, config init/edit\nUse COMMAND --help for its flags.")
		}
	}
	fs.StringVar(&args.ConfigFile, "config", defaultFile, "Config file path")
	fs.BoolVar(&args.Debug, "debug", false, "Write private rotating debug log")
	fs.BoolVar(&args.Force, "force", false, "Force refresh, ignore cache")
	fs.DurationVar(&args.TTL, "ttl", time.Hour, "Cache TTL duration")
	fs.StringVar(&args.Profile, "profile", "", "Named output profile")
	args.Output = "shell"
	if !args.Check {
		fs.BoolVar(&args.Clean, "clean", false, "Clean all scoped caches in selected backend")
		fs.StringVar(&args.Output, "output", "shell", "Output format: shell, json, env")
		fs.BoolVar(&args.Version, "version", false, "Print version information")
	}
	err := fs.Parse(argv)
	if errors.Is(err, flag.ErrHelp) {
		if err := writeOutput(stderr, []byte(parseOutput.String())); err != nil {
			return args, errors.New("help output write failed")
		}
		return args, flag.ErrHelp
	}
	if err != nil {
		return args, errors.New("invalid flags; use --help for usage")
	}
	if fs.NArg() != 0 {
		return args, errors.New("unexpected command argument; use --help for usage")
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "profile" {
			args.ProfileSet = true
		}
	})
	if args.ProfileSet && args.Profile == "" {
		return args, errors.New("--profile must not be empty")
	}
	if args.Clean && args.ProfileSet {
		return args, errors.New("--clean cannot be combined with --profile")
	}
	if args.Check && !args.ProfileSet {
		return args, errors.New("--profile is required for profile check")
	}
	if !output.ValidFormat(args.Output) {
		return args, errors.New("output must be shell, json, or env")
	}
	return args, nil
}

func main() { os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr)) }

func runCLI(argv []string, stdout, stderr io.Writer) int {
	if len(argv) > 0 {
		switch argv[0] {
		case "run":
			return runCommand(argv[1:], stdout, stderr)
		case "credential":
			return credentialCommand(argv[1:], stdout, stderr)
		case "store":
			return storeCommand(argv[1:], stdout, stderr)
		}
	}
	if len(argv) > 1 && argv[0] == "profile" && (argv[1] == "list" || argv[1] == "describe") {
		return runProfileInspection(argv[1], argv[2:], stdout, stderr)
	}
	if len(argv) > 0 && argv[0] == "config" {
		if err := runConfigCommand(argv[1:], stdout, stderr); err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
		return 0
	}
	args, err := parseArgs(argv, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	if args.Version {
		version := fmt.Sprintf(
			"secret_inject version %s\nCommit: %s\nBuild date: %s\n",
			Version,
			GitCommit,
			BuildDate,
		)
		if err := writeOutput(stdout, []byte(version)); err != nil {
			fmt.Fprintln(stderr, "Error: output write failed")
			return 1
		}
		return 0
	}
	var debug *diagnostics.Session
	if args.Debug {
		debug, err = diagnostics.Open()
		if err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
	}
	ctx, stop := commandContext()
	defer stop()
	data, code, err := execute(ctx, args, debug)
	if err != nil {
		debug.Record(diagnostics.Event{Kind: diagnostics.Failure, Operation: "command"})
	}
	if logErr := debug.Finish(); logErr != nil {
		fmt.Fprintln(stderr, "Error:", logErr)
		return 1
	}
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	if err := writeOutput(stdout, data); err != nil {
		fmt.Fprintln(stderr, "Error: output write failed")
		return 1
	}
	return code
}

func writeOutput(w io.Writer, data []byte) error {
	n, err := w.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

func execute(ctx context.Context, args Args, debug *diagnostics.Session) (data []byte, code int, err error) {
	cfg, err := config.ReadConfig(args.ConfigFile)
	if err != nil {
		return nil, 1, errors.New("configuration could not be read or decoded")
	}
	var requested *string
	if args.ProfileSet {
		requested = &args.Profile
	}
	selected, selectedErr := cfg.Profiles.Choose(requested)
	if !args.Clean && selectedErr != nil {
		return nil, 1, selectedErr
	}
	if args.Clean || selected.RemoteSources() {
		if err := cfg.Validate(); err != nil {
			return nil, 1, err
		}
	}
	if args.ProfileSet {
		debug.Record(diagnostics.Event{Kind: diagnostics.ProfileSelected, Profile: args.Profile})
	}
	if args.Clean {
		debug.Record(diagnostics.Event{Kind: diagnostics.StorageProgress, Operation: "cleanup", State: "started"})
		if err := cleanConfiguredCache(ctx, cfg); err != nil {
			return nil, 1, errors.New("cache cleanup failed")
		}
		debug.Record(diagnostics.Event{Kind: diagnostics.StorageProgress, Operation: "cleanup", State: "completed"})
		debug.Record(diagnostics.Event{Kind: diagnostics.ResultPrepared, Operation: "clean"})
		return []byte("Cached secrets cleaned successfully\n"), 0, nil
	}
	resolved, err := loadSnapshot(ctx, cfg, args, selected, debug)
	if err != nil {
		return nil, 1, err
	}
	defer func() {
		err = errors.Join(err, resolved.Close())
		if err != nil {
			data = nil
			code = 1
		}
	}()
	if args.Check {
		report, events := selected.Check(args.Profile, resolved.Values)
		debug.RecordSelection(events)
		data, err := json.Marshal(report)
		if err != nil {
			return nil, 1, errors.New("report encoding failed")
		}
		code := 0
		if len(report.MissingIncludes) > 0 {
			code = 2
		}
		debug.Record(diagnostics.Event{
			Kind:      diagnostics.ResultPrepared,
			Operation: "check",
			Count:     len(report.MissingIncludes),
		})
		if err := resolved.Finish(ctx); err != nil {
			return nil, 1, err
		}
		return append(data, '\n'), code, nil
	}
	projected, events, err := selected.Project(resolved.Values)
	debug.RecordSelection(events)
	if err != nil {
		return nil, 1, err
	}
	data, err = output.Render(projected, args.Output, selected.Overwrite())
	if err != nil {
		return nil, 1, err
	}
	debug.Record(diagnostics.Event{Kind: diagnostics.ResultPrepared, Operation: "export", Count: len(projected.Entries)})
	if err := resolved.Finish(ctx); err != nil {
		return nil, 1, err
	}
	return data, 0, nil
}
