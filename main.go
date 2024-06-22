package main

import (
	"flag"
	"log/slog"
	"os"
)

type Args struct {
	ConfigFile string
	Clean      bool
  Debug      bool
}

var defaultFile = os.Getenv("HOME") + string(os.PathSeparator) + ".config" + string(os.PathSeparator) + ".secret_inject.json"

func parseArgs() Args {
	var args Args
	flag.StringVar(&args.ConfigFile, "config", defaultFile, "Config file path")
	flag.BoolVar(&args.Clean, "clean", false, "Clean cached secrets")
  flag.BoolVar(&args.Debug, "debug", false, "debug")
	flag.Parse()
	return args
}

func main() {
	args := parseArgs()
	sources := []SecretSource{}
	sources = append(sources, NewSourceDoppler())
  if args.Debug {
    slog.SetLogLoggerLevel(slog.LevelDebug)
  }

	fullConfig, err := ReadFullConfig(args.ConfigFile)
	if err != nil {
		slog.Error("Error reading config file: %s", args.ConfigFile)
		panic(err)
	}

	storage, err := GetStorage(fullConfig)
  if err != nil {
    slog.Error("Error getting storage: %s", err)
    panic(err)
  }

	if args.Clean {
		slog.Debug("Cleaning cached secrets")
		err := storage.CleanCachedSecrets()
		if err != nil {
			slog.Error("Error cleaning cached secrets: %s", err)
			panic(err)
		}
		return
	}

	secrets := NewSecrets()

	if storage.HasCachedSecrets() {
		slog.Debug("Found cached secrets")
		secrets, err = storage.GetCachedSecrets()
		if err != nil {
			slog.Error("Error getting cached secrets: %s", err)
			panic(err)
		}

	} else {
		slog.Debug("No cached secrets found")
		for _, source := range sources {
			err := source.Init(fullConfig)
			if err != nil {
				slog.Error("Error initializing source: %s", source)
				panic(err)
			}
			moreSecrets, err := source.GetAllSecrets()
			if err != nil {
				slog.Error("Error getting secrets: %s", err)
				panic(err)
			}
      secrets = secrets.AppendSecrets(moreSecrets)
		}

	  err = storage.CacheSecrets(secrets)
    if err != nil {
      slog.Error("Error caching secrets: %s", err)
      panic(err)
    }
	}


  ExportAsEnvVars(secrets)
}
