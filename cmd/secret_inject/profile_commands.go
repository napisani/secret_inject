package main

import (
	"errors"
	"flag"
	"io"
	"sort"
	"strings"

	"github.com/napisani/secret_inject/internal/config"
	"github.com/napisani/secret_inject/internal/profile"
	"github.com/napisani/secret_inject/internal/source"
)

type profileListResponse struct {
	SchemaVersion int      `json:"schema_version"`
	Profiles      []string `json:"profiles"`
}

type resolutionMetadata struct {
	Kind string `json:"kind"`
}

type profileDescription struct {
	profile.Description
	DeclaredSourceOrder []string           `json:"declared_source_order"`
	Resolution          resolutionMetadata `json:"resolution"`
}

type profileDescriptionResponse struct {
	SchemaVersion int                `json:"schema_version"`
	Profile       profileDescription `json:"profile"`
}

func runProfileInspection(command string, argv []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("secret_inject profile "+command, flag.ContinueOnError)
	var parseOutput strings.Builder
	fs.SetOutput(&parseOutput)
	configFile := fs.String("config", defaultFile, "Config file path")
	var profileName string
	if command == "describe" {
		fs.StringVar(&profileName, "profile", "", "Named output profile")
	}
	if err := fs.Parse(argv); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			if err := writeOutput(stderr, []byte(parseOutput.String())); err != nil {
				return 1
			}
			return 0
		}
		return reportCommandFailure(stderr, failureUsage)
	}
	if fs.NArg() != 0 || (command == "describe" && profileName == "") {
		return reportCommandFailure(stderr, failureUsage)
	}
	cfg, err := config.ReadConfig(*configFile)
	if err != nil {
		return reportCommandFailure(stderr, failureConfiguration)
	}

	var response any
	switch command {
	case "list":
		profiles := make([]string, 0, len(cfg.Profiles))
		for name := range cfg.Profiles {
			profiles = append(profiles, name)
		}
		sort.Strings(profiles)
		response = profileListResponse{SchemaVersion: 1, Profiles: profiles}
	case "describe":
		selected, err := cfg.Profiles.Choose(&profileName)
		if err != nil {
			return reportCommandFailure(stderr, failureProfileSelection)
		}
		order := []string{}
		if selected.RemoteSources() {
			order, err = source.DeclaredSourceOrder(cfg.Sources, cfg.SourceSequence)
			if err != nil {
				return reportCommandFailure(stderr, failureConfiguration)
			}
		}
		response = profileDescriptionResponse{
			SchemaVersion: 1,
			Profile: profileDescription{
				Description:         selected.Describe(profileName),
				DeclaredSourceOrder: order,
				Resolution:          resolutionMetadata{Kind: "not_evaluated"},
			},
		}
	default:
		return reportCommandFailure(stderr, failureUsage)
	}
	if err := writeJSON(stdout, response); err != nil {
		return reportCommandFailure(stderr, failureMetadataOutput)
	}
	return 0
}
