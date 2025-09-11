// hash-persister is a binary to compute and persist target hashes for a given git commit SHA.
// This allows for later comparison between commits without recomputing hashes.

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
)

type hashPersisterFlags struct {
	commonFlags *cli.CommonFlags
	commitSha   string
	outputFile  string
}

type config struct {
	Context    *pkg.Context
	CommitSha  string
	Targets    pkg.TargetsList
	OutputFile string
}

func main() {
	start := time.Now()
	defer func() { log.Printf("Finished after %v", time.Since(start)) }()

	flags, err := parseFlags()
	if err != nil {
		fmt.Fprintf(flag.CommandLine.Output(), "Failed to parse flags: %v\n", err)
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s <git-commit-sha>\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(flag.CommandLine.Output(), "Where <git-commit-sha> is the commit SHA to compute and persist hashes for.\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Optional flags:\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	config, err := resolveConfig(*flags)
	if err != nil {
		fmt.Println("Hash Persister invocation Error")
		log.Fatalf("Error during preprocessing: %v", err)
	}

	// Create LabelledGitRev for the specified commit
	commitRev, err := pkg.NewLabelledGitRev(config.Context.WorkspacePath, config.CommitSha, "commit")
	if err != nil {
		log.Fatalf("Failed to resolve commit %s: %v", config.CommitSha, err)
	}

	log.Printf("Computing hashes for commit %s", config.CommitSha)

	// Process the commit to get query results with computed hashes
	queryResults, cleanup, err := pkg.LoadIncompleteMetadata(config.Context, commitRev, config.Targets)
	defer cleanup()
	if err != nil {
		log.Fatalf("Failed to load metadata for commit %s: %v", config.CommitSha, err)
	}

	log.Println("Computing target hashes")
	if err := queryResults.PrefillCache(); err != nil {
		log.Fatalf("Failed to compute hashes for commit %s: %v", config.CommitSha, err)
	}

	log.Printf("Persisting hashes to %s", config.OutputFile)
	if err := pkg.PersistHashes(config.OutputFile, config.CommitSha, queryResults, config.Context, config.Targets.String()); err != nil {
		log.Fatalf("Failed to persist hashes: %v", err)
	}

	log.Printf("Successfully persisted hashes for %d targets to %s",
		len(queryResults.MatchingTargets.Labels()), config.OutputFile)
}

func parseFlags() (*hashPersisterFlags, error) {
	var flags hashPersisterFlags
	flags.commonFlags = cli.RegisterCommonFlags()
	flag.StringVar(&flags.outputFile, "output", "", "Output file path for persisted hashes (required)")

	flag.Parse()

	if flags.outputFile == "" {
		return nil, fmt.Errorf("output file is required")
	}

	positional := flag.Args()
	if len(positional) != 1 {
		return nil, fmt.Errorf("expected one positional argument, <git-commit-sha>, but got %d", len(positional))
	}
	flags.commitSha = positional[0]

	return &flags, nil
}

func resolveConfig(flags hashPersisterFlags) (*config, error) {
	// Validate working directory
	workingDirectory, err := filepath.Abs(*flags.commonFlags.WorkingDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory from %v: %w", *flags.commonFlags.WorkingDirectory, err)
	}

	// Get current revision to restore later
	currentBranch, err := pkg.GitRevParse(workingDirectory, "HEAD", true)
	if err != nil {
		return nil, fmt.Errorf("failed to get current git revision: %w", err)
	}

	afterRev, err := pkg.NewLabelledGitRev(workingDirectory, currentBranch, "current")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve the current git revision: %w", err)
	}

	bazelCmd := pkg.DefaultBazelCmd{
		BazelPath:        *flags.commonFlags.BazelPath,
		BazelStartupOpts: *flags.commonFlags.BazelStartupOpts,
		BazelOpts:        *flags.commonFlags.BazelOpts,
	}

	outputBase, err := pkg.BazelOutputBase(workingDirectory, bazelCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve the bazel output base: %w", err)
	}

	context := &pkg.Context{
		WorkspacePath:                          workingDirectory,
		OriginalRevision:                       afterRev,
		BazelCmd:                               bazelCmd,
		BazelOutputBase:                        outputBase,
		DeleteCachedWorktree:                   flags.commonFlags.DeleteCachedWorktree,
		IgnoredFiles:                           *flags.commonFlags.IgnoredFiles,
		BeforeQueryErrorBehavior:               *flags.commonFlags.BeforeQueryErrorBehavior,
		AnalysisCacheClearStrategy:             *flags.commonFlags.AnalysisCacheClearStrategy,
		CompareQueriesAroundAnalysisCacheClear: flags.commonFlags.CompareQueriesAroundAnalysisCacheClear,
		FilterIncompatibleTargets:              flags.commonFlags.FilterIncompatibleTargets,
		EnforceCleanRepo:                       flags.commonFlags.EnforceCleanRepo == cli.EnforceClean,
	}

	targetsList, err := pkg.ParseTargetsList(*flags.commonFlags.TargetsFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to parse targets: %w", err)
	}

	// Validate output file directory exists
	outputDir := filepath.Dir(flags.outputFile)
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
		}
	}

	return &config{
		Context:    context,
		CommitSha:  flags.commitSha,
		Targets:    targetsList,
		OutputFile: flags.outputFile,
	}, nil
}
