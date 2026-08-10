package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path"
	"time"

	"github.com/napisani/secret_inject/internal/config"
	"github.com/napisani/secret_inject/internal/output"
	"github.com/napisani/secret_inject/internal/secret"
	"github.com/napisani/secret_inject/internal/source"
	"github.com/napisani/secret_inject/internal/storage"
)

type Args struct {
	ConfigFile string
	Clean      bool
	Debug      bool
	Force      bool
	TTL        time.Duration
	Output     string
	Version    bool
}

var defaultFile = path.Join(os.Getenv("HOME"), ".config", ".secret_inject.json")

// Version information (set via ldflags)
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func parseArgs() Args {
	var args Args
	flag.StringVar(&args.ConfigFile, "config", defaultFile, "Config file path")
	flag.BoolVar(&args.Clean, "clean", false, "Clean cached secrets")
	flag.BoolVar(&args.Debug, "debug", false, "Enable debug logging")
	flag.BoolVar(&args.Force, "force", false, "Force refresh, ignore cache")
	flag.DurationVar(&args.TTL, "ttl", 1*time.Hour, "Cache TTL duration (e.g., '1h', '30m')")
	flag.StringVar(&args.Output, "output", "shell", "Output format: shell, json, env")
	flag.BoolVar(&args.Version, "version", false, "Print version information")
	flag.Parse()
	return args
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "config" {
		if err := runConfigCommand(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		return
	}

	args := parseArgs()

	// Handle version flag
	if args.Version {
		fmt.Printf("secret_inject version %s\nCommit: %s\nBuild date: %s\n", Version, GitCommit, BuildDate)
		return
	}

	// Configure logging
	if args.Debug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	// Read and validate config
	cfg, err := config.ReadConfig(args.ConfigFile)
	if err != nil {
		slog.Error("Error reading config file", "file", args.ConfigFile, "error", err)
		os.Exit(1)
	}

	if err := cfg.Validate(); err != nil {
		slog.Error("Invalid config", "error", err)
		os.Exit(1)
	}

	// Initialize storage
	stor, err := storage.Get(cfg.Storage)
	if err != nil {
		slog.Error("Error getting storage", "error", err)
		os.Exit(1)
	}

	// Handle clean command
	if args.Clean {
		slog.Debug("Cleaning cached secrets")
		err := stor.CleanCachedSecrets()
		if err != nil {
			slog.Error("Error cleaning cached secrets", "error", err)
			os.Exit(1)
		}
		fmt.Println("Cached secrets cleaned successfully")
		return
	}

	// Initialize sources
	fullConfig := make(map[string]interface{})
	fullConfig["sources"] = cfg.Sources
	fullConfig["storage"] = cfg.Storage
	if len(cfg.SourceSequence) > 0 {
		fullConfig["source_sequence"] = cfg.SourceSequence
	}

	sources, err := source.LoadAll(fullConfig)
	if err != nil {
		slog.Error("Error loading sources", "error", err)
		os.Exit(1)
	}

	if len(sources) == 0 {
		slog.Debug("No sources enabled, continuing with empty secrets")
	}

	secrets := secret.New()

	// Check if we should use cached secrets
	useCached := stor.HasCachedSecrets() && !args.Force
	if useCached {
		slog.Debug("Found cached secrets")
		secrets, err = stor.GetCachedSecrets()
		if err != nil {
			slog.Error("Error getting cached secrets", "error", err)
			os.Exit(1)
		}

		// Check if cache is expired
		if secrets.IsExpired(args.TTL) {
			slog.Debug("Cached secrets expired", "age", time.Since(secrets.Timestamp))
			useCached = false
		}
	}

	// Fetch from sources if not using cache
	if !useCached {
		slog.Debug("Fetching secrets from sources")

		for _, src := range sources {
			// Check if source is enabled
			if !src.IsEnabled() {
				slog.Debug("Source disabled, skipping")
				continue
			}

			moreSecrets, err := src.GetAllSecrets(secrets)
			if err != nil {
				slog.Error("Error getting secrets", "error", err)
				os.Exit(1)
			}
			secrets = secrets.Append(moreSecrets)
		}

		// Cache the secrets
		err = stor.CacheSecrets(secrets)
		if err != nil {
			slog.Error("Error caching secrets", "error", err)
			os.Exit(1)
		}
	}

	// Export secrets
	output.Export(secrets, args.Output)
}
