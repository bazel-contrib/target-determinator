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

type config struct {
	RevisionBefore pkg.LabelledGitRev
	RevisionAfter  pkg.LabelledGitRev
	Context        *pkg.Context
	Verbose        bool
	TargetPattern  gazelle_label.Pattern
}

func main() {
	start := time.Now()
	defer func() { log.Printf("Finished after %v", time.Since(start)) }()

	config, err := parseFlags()
	if err != nil {
		fmt.Fprintf(flag.CommandLine.Output(), "Failed to parse flags: %v\n", err)
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s <before-revision>\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(flag.CommandLine.Output(), "Where <before-revision> may be any commit revision - full commit hashes, short commit hashes, tags, branches, etc.\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Optional flags:\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	callback := func(label gazelle_label.Label, differences []pkg.Difference, configuredTarget *analysis.ConfiguredTarget) {
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
	}

	if err := pkg.WalkAffectedTargets(config.Context,
		config.RevisionBefore,
		config.TargetPattern,
		config.Verbose,
		callback); err != nil {
		// Print all targets to stdout in case the caller doesn't check for exit codes (e.g. using pipes in the shell).
		fmt.Println(config.TargetPattern.String())
		log.Fatal(err)
	}
}

func parseFlags() (*config, error) {
	commonFlags := cli.RegisterCommonFlags()
	targetPatternFlag := flag.String("target-pattern", "//...", "Target pattern to diff.")
	verbose := flag.Bool("verbose", false, "Whether to explain (messily) why each target is getting run")

	flag.Parse()

	commonArgs, err := cli.ProcessCommonArgs(commonFlags, targetPatternFlag)
	if err != nil {
		return nil, err
	}

	return &config{
		RevisionBefore: commonArgs.RevisionBefore,
		Context:        commonArgs.Context,
		Verbose:        *verbose,
		TargetPattern:  commonArgs.TargetPattern,
	}, nil
}
