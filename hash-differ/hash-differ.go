// hash-differ is a binary to compare two persisted hash files and identify differences.
// This allows for efficient Bazel diff computation without recomputing hashes.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/bazel-contrib/target-determinator/pkg"
)

type hashDifferFlags struct {
	beforeFile   string
	afterFile    string
	outputFormat string
	outputFile   string
	verbose      bool
}

func main() {
	flags, err := parseFlags()
	if err != nil {
		fmt.Fprintf(flag.CommandLine.Output(), "Failed to parse flags: %v\n", err)
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s <before-hash-file> <after-hash-file>\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(flag.CommandLine.Output(), "Where:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  <before-hash-file> is the path to the first hash file\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  <after-hash-file> is the path to the second hash file\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Optional flags:\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	log.Printf("Comparing hash files: %s vs %s", flags.beforeFile, flags.afterFile)

	result, err := pkg.CompareHashFiles(flags.beforeFile, flags.afterFile)
	if err != nil {
		log.Fatalf("Failed to compare hash files: %v", err)
	}

	if flags.verbose {
		log.Printf("Comparison complete:")
		log.Printf("  Before commit: %s", result.BeforeCommit)
		log.Printf("  After commit: %s", result.AfterCommit)
		log.Printf("  Total differences: %d", len(result.Differences))
		log.Printf("  Changed targets: %d", result.Summary.TotalChanged)
		log.Printf("  Added targets: %d", result.Summary.TotalAdded)
		log.Printf("  Removed targets: %d", result.Summary.TotalRemoved)
		log.Printf("  Affected target labels: %d", len(result.Summary.AffectedTargets))
	}

	switch flags.outputFormat {
	case "json":
		err = outputJSON(result, flags.outputFile)
	case "targets":
		err = outputTargetList(result, flags.outputFile)
	case "summary":
		err = outputSummary(result, flags.outputFile)
	default:
		err = fmt.Errorf("unsupported output format: %s", flags.outputFormat)
	}

	if err != nil {
		log.Fatalf("Failed to output results: %v", err)
	}

	if flags.outputFile == "" {
		log.Printf("Results written to stdout")
	} else {
		log.Printf("Results written to %s", flags.outputFile)
	}
}

func parseFlags() (*hashDifferFlags, error) {
	var flags hashDifferFlags

	flag.StringVar(&flags.outputFormat, "format", "targets", "Output format: json, targets, or summary")
	flag.StringVar(&flags.outputFile, "output", "", "Output file (default: stdout)")
	flag.BoolVar(&flags.verbose, "verbose", false, "Enable verbose logging")

	flag.Parse()

	positional := flag.Args()
	if len(positional) != 2 {
		return nil, fmt.Errorf("expected two positional arguments, <before-hash-file> <after-hash-file>, but got %d", len(positional))
	}

	flags.beforeFile = positional[0]
	flags.afterFile = positional[1]

	// Validate input files exist
	if _, err := os.Stat(flags.beforeFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("before hash file does not exist: %s", flags.beforeFile)
	}
	if _, err := os.Stat(flags.afterFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("after hash file does not exist: %s", flags.afterFile)
	}

	// Validate output format
	validFormats := map[string]bool{
		"json":    true,
		"targets": true,
		"summary": true,
	}
	if !validFormats[flags.outputFormat] {
		return nil, fmt.Errorf("invalid output format: %s (valid options: json, targets, summary)", flags.outputFormat)
	}

	return &flags, nil
}

func outputJSON(result *pkg.HashComparisonResult, outputFile string) error {
	var output *os.File
	var err error

	if outputFile == "" {
		output = os.Stdout
	} else {
		output, err = os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer output.Close()
	}

	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func outputTargetList(result *pkg.HashComparisonResult, outputFile string) error {
	var output *os.File
	var err error

	if outputFile == "" {
		output = os.Stdout
	} else {
		output, err = os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer output.Close()
	}

	// Output one target label per line (compatible with target-determinator output)
	for _, target := range result.Summary.AffectedTargets {
		if _, err := fmt.Fprintln(output, target); err != nil {
			return fmt.Errorf("failed to write target: %w", err)
		}
	}

	return nil
}

func outputSummary(result *pkg.HashComparisonResult, outputFile string) error {
	var output *os.File
	var err error

	if outputFile == "" {
		output = os.Stdout
	} else {
		output, err = os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer output.Close()
	}

	fmt.Fprintf(output, "Hash Comparison Summary\n")
	fmt.Fprintf(output, "======================\n")
	fmt.Fprintf(output, "Before commit: %s\n", result.BeforeCommit)
	fmt.Fprintf(output, "After commit:  %s\n", result.AfterCommit)
	fmt.Fprintf(output, "\n")
	fmt.Fprintf(output, "Target Changes:\n")
	fmt.Fprintf(output, "  Changed:     %d\n", result.Summary.TotalChanged)
	fmt.Fprintf(output, "  Added:       %d\n", result.Summary.TotalAdded)
	fmt.Fprintf(output, "  Removed:     %d\n", result.Summary.TotalRemoved)
	fmt.Fprintf(output, "  Total Diff:  %d\n", len(result.Differences))
	fmt.Fprintf(output, "\n")
	fmt.Fprintf(output, "Affected Target Labels (%d):\n", len(result.Summary.AffectedTargets))
	for _, target := range result.Summary.AffectedTargets {
		fmt.Fprintf(output, "  %s\n", target)
	}

	return nil
}
