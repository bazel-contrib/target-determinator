package cli

import (
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/bazel-contrib/target-determinator/common"
	"github.com/bazel-contrib/target-determinator/pkg"
	gazelle_label "github.com/bazelbuild/bazel-gazelle/label"
)

type IgnoreFileFlag []common.RelPath

func (i *IgnoreFileFlag) stringSlice() []string {
	var stringSlice []string
	for _, p := range *i {
		stringSlice = append(stringSlice, p.String(), "/")
	}
	return stringSlice
}

func (i *IgnoreFileFlag) String() string {
	return "[" + strings.Join(i.stringSlice(), ", ") + "]"
}

func (i *IgnoreFileFlag) Set(value string) error {
	*i = append(*i, common.NewRelPath(value))
	return nil
}

type CommonFlags struct {
	WorkingDirectory *string
	BazelPath        *string
	IgnoredFiles     *IgnoreFileFlag
}

func StrPtr() *string {
	var s string
	return &s
}

func RegisterCommonFlags() *CommonFlags {
	commonFlags := CommonFlags{
		WorkingDirectory: StrPtr(),
		BazelPath:        StrPtr(),
		IgnoredFiles:     &IgnoreFileFlag{},
	}
	flag.StringVar(commonFlags.WorkingDirectory, "working-directory", ".", "Working directory to query")
	flag.StringVar(commonFlags.BazelPath, "bazel", "bazel",
		"Bazel binary (basename on $PATH, or absolute or relative path) to run")
	flag.Var(commonFlags.IgnoredFiles, "ignore-file",
		"Files to ignore for git operations, relative to the working-directory. These files shan't affect the Bazel graph.")
	return &commonFlags
}

type ProcessedCommonArgs struct {
	Context        *pkg.Context
	RevisionBefore pkg.LabelledGitRev
	RevisionAfter  pkg.LabelledGitRev
	TargetPattern  gazelle_label.Pattern
}

func ProcessCommonArgs(commonFlags *CommonFlags, targetPatternFlag *string) (*ProcessedCommonArgs, error) {
	workingDirectory, err := filepath.Abs(*commonFlags.WorkingDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory from %v: %w", *commonFlags.WorkingDirectory, err)
	}

	currentBranch, err := pkg.GitRevParse(workingDirectory, "HEAD", true)
	if err != nil {
		return nil, fmt.Errorf("failed to get current git revision: %w", err)
	}
	afterRev, err := pkg.NewLabelledGitRev(workingDirectory, currentBranch, "after")
	if err != nil {
		log.Fatalf("Error while resolving the \"after\" (i.e. original) git revision: %v", err)
	}

	context := &pkg.Context{
		OriginalWorkspacePath: workingDirectory,
		OriginalRevision:      afterRev,
		BazelPath:             *commonFlags.BazelPath,
		IgnoredFiles:          *commonFlags.IgnoredFiles,
	}

	positional := flag.Args()
	if len(positional) != 1 {
		return nil, fmt.Errorf("expected one positional argument, <before-revision>, but got %d", len(positional))
	}

	beforeRev, err := pkg.NewLabelledGitRev(workingDirectory, positional[0], "before")
	if err != nil {
		log.Fatalf("Error while resolving the \"before\" git revision: %v", err)
	}

	targetPattern, err := gazelle_label.ParsePattern(*targetPatternFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to parse -target-pattern: %w", err)
	}
	return &ProcessedCommonArgs{
		Context:        context,
		RevisionBefore: beforeRev,
		RevisionAfter:  afterRev,
		TargetPattern:  targetPattern,
	}, nil
}
