package cli

import (
	"flag"
	"fmt"
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

type EnforceCleanFlag int

const (
	EnforceClean EnforceCleanFlag = iota
	AllowIgnored
	AllowDirty
)

func (s EnforceCleanFlag) String() string {
	switch s {
	case EnforceClean:
		return "enforce-clean"
	case AllowIgnored:
		return "allow-ignored"
	case AllowDirty:
		return "allow-dirty"
	}
	return ""
}

func (i *EnforceCleanFlag) Set(value string) error {
	switch value {
	case "enforce-clean":
		*i = EnforceClean
	case "allow-ignored":
		*i = AllowIgnored
	case "allow-dirty":
		*i = AllowDirty
	default:
		return fmt.Errorf("invalid value for --enforce-clean: %v", value)
	}
	return nil
}

type CommonFlags struct {
	WorkingDirectory  *string
	BazelPath         *string
	EnforceCleanRepo  EnforceCleanFlag
	IgnoredFiles      *IgnoreFileFlag
	TargetPatternFlag *string
}

func StrPtr() *string {
	var s string
	return &s
}

func RegisterCommonFlags() *CommonFlags {
	commonFlags := CommonFlags{
		WorkingDirectory:  StrPtr(),
		BazelPath:         StrPtr(),
		EnforceCleanRepo:  AllowIgnored,
		IgnoredFiles:      &IgnoreFileFlag{},
		TargetPatternFlag: StrPtr(),
	}
	flag.StringVar(commonFlags.WorkingDirectory, "working-directory", ".", "Working directory to query")
	flag.StringVar(commonFlags.BazelPath, "bazel", "bazel",
		"Bazel binary (basename on $PATH, or absolute or relative path) to run")
	flag.Var(&commonFlags.EnforceCleanRepo, "enforce-clean",
		fmt.Sprintf("Pass --enforce-clean=%v to fail if the repository is unclean, or --enforce-clean=%v to allow ignored untracked files (the default).",
			EnforceClean.String(), AllowIgnored.String()))
	flag.Var(commonFlags.IgnoredFiles, "ignore-file",
		"Files to ignore for git operations, relative to the working-directory. These files shan't affect the Bazel graph.")
	flag.StringVar(commonFlags.TargetPatternFlag, "target-pattern", "//...", "Target pattern to diff.")
	return &commonFlags
}

type CommonConfig struct {
	Context        *pkg.Context
	RevisionBefore pkg.LabelledGitRev
	TargetPattern  gazelle_label.Pattern
}

// ValidateCommonFlags ensures that the argument follow the right format
func ValidateCommonFlags() (targetPattern string, err error) {
	positional := flag.Args()
	if len(positional) != 1 {
		return "", fmt.Errorf("expected one positional argument, <before-revision>, but got %d", len(positional))
	}
	return positional[0], nil

}

func ResolveCommonConfig(commonFlags *CommonFlags, beforeRevStr string) (*CommonConfig, error) {

	// Context attributes

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
		return nil, fmt.Errorf("failed to resolve the \"after\" (i.e. original) git revision: %w", err)
	}

	outputBase, err := pkg.BazelOutputBase(*commonFlags.BazelPath, workingDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve the bazel output base: %w", err)
	}

	context := &pkg.Context{
		WorkspacePath:    workingDirectory,
		OriginalRevision: afterRev,
		BazelPath:        *commonFlags.BazelPath,
		BazelOutputBase:  outputBase,
		IgnoredFiles:     *commonFlags.IgnoredFiles,
	}

	// Non-context attributes

	beforeRev, err := pkg.NewLabelledGitRev(workingDirectory, beforeRevStr, "before")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve the \"before\" git revision: %w", err)
	}

	targetPattern, err := gazelle_label.ParsePattern(*commonFlags.TargetPatternFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to parse target pattern: %w", err)
	}

	// Additional checks

	isCleanRepo, err := pkg.EnsureGitRepositoryClean(workingDirectory, *commonFlags.IgnoredFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to check whether the repository is clean: %w", err)
	}

	if !isCleanRepo && commonFlags.EnforceCleanRepo == EnforceClean {
		return nil, fmt.Errorf("current repository is not clean and --enforce-clean option is set to '%v'. Exiting.", EnforceClean.String())
	}

	return &CommonConfig{
		Context:        context,
		RevisionBefore: beforeRev,
		TargetPattern:  targetPattern,
	}, nil
}
