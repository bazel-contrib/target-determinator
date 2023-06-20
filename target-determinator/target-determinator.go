// target-determinator is a binary to output to stdout a list of targets, one-per-line, which may
// have changed between two commits.
// It goes to some efforts to be both thorough, and minimal, but if in doubt leans towards
// over-building rather than under-building.
// In verbose mode, the first token per line will be the target to run, and after a space character,
// additional information may be printed explaining why a target was detected to be affected.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/bazel-contrib/target-determinator/cli"
	"github.com/bazel-contrib/target-determinator/pkg"
	"github.com/bazel-contrib/target-determinator/third_party/protobuf/bazel/analysis"
	gazelle_label "github.com/bazelbuild/bazel-gazelle/label"
)

type targetDeterminatorFlags struct {
	commonFlags    *cli.CommonFlags
	revisionBefore string
	verbose        bool
}

type config struct {
	Context        *pkg.Context
	RevisionBefore pkg.LabelledGitRev
	Targets        pkg.TargetsList
	Verbose        bool
}

func main() {
	start := time.Now()
	defer func() { log.Printf("Finished after %v", time.Since(start)) }()

	flags, err := parseFlags()
	if err != nil {
		fmt.Fprintf(flag.CommandLine.Output(), "Failed to parse flags: %v\n", err)
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s <before-revision>\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(flag.CommandLine.Output(), "Where <before-revision> may be any commit revision - full commit hashes, short commit hashes, tags, branches, etc.\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Optional flags:\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Print something on stdout that will make bazel fail when passed as a target.
	config, err := resolveConfig(*flags)
	if err != nil {
		fmt.Println("Target Determinator invocation Error")
		log.Fatalf("Error during preprocessing: %v", err)
	}

	seenLabels := make(map[gazelle_label.Label]struct{})
	callback := func(label gazelle_label.Label, differences []pkg.Difference, configuredTarget *analysis.ConfiguredTarget) {
		if !config.Verbose {
			if _, seen := seenLabels[label]; seen {
				return
			}
		}
		fmt.Print(label)
		if len(differences) > 0 {
			fmt.Printf(" Changes:")
			for i, difference := range differences {
				if i > 0 {
					fmt.Print(",")
				}
				fmt.Printf(" %v", difference.String())
			}
		}
		fmt.Println("")
		seenLabels[label] = struct{}{}
	}

	if err := pkg.WalkAffectedTargets(config.Context,
		config.RevisionBefore,
		config.Targets,
		config.Verbose,
		callback); err != nil {
		// Print something on stdout that will make bazel fail when passed as a target.
		fmt.Println("Target Determinator invocation Error")
		log.Fatal(err)
	}
}

func parseFlags() (*targetDeterminatorFlags, error) {
	var flags targetDeterminatorFlags
	flags.commonFlags = cli.RegisterCommonFlags()
	flag.BoolVar(&flags.verbose, "verbose", false, "Whether to explain (messily) why each target is getting run")

	flag.Parse()

	var err error
	flags.revisionBefore, err = cli.ValidateCommonFlags("target-determinator", flags.commonFlags)
	if err != nil {
		return nil, err
	}
	return &flags, nil
}

func resolveConfig(flags targetDeterminatorFlags) (*config, error) {
	commonArgs, err := cli.ResolveCommonConfig(flags.commonFlags, flags.revisionBefore)
	if err != nil {
		return nil, err
	}

	return &config{
		Context:        commonArgs.Context,
		RevisionBefore: commonArgs.RevisionBefore,
		Targets:        commonArgs.Targets,
		Verbose:        flags.verbose,
	}, nil
}
