package cli

import (
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/bazel-contrib/target-determinator/common"
	"github.com/bazel-contrib/target-determinator/pkg"
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

func (e EnforceCleanFlag) String() string {
	switch e {
	case EnforceClean:
		return "enforce-clean"
	case AllowIgnored:
		return "allow-ignored"
	case AllowDirty:
		return "allow-dirty"
	}
	return ""
}

func (e *EnforceCleanFlag) Set(value string) error {
	switch value {
	case "enforce-clean":
		*e = EnforceClean
	case "allow-ignored":
		*e = AllowIgnored
	case "allow-dirty":
		*e = AllowDirty
	default:
		return fmt.Errorf("invalid value for --enforce-clean: %v", value)
	}
	return nil
}

type CommonFlags struct {
	WorkingDirectory                       *string
	BazelPath                              *string
	BazelStartupOpts                       *MultipleStrings
	BazelOpts                              *MultipleStrings
	EnforceCleanRepo                       EnforceCleanFlag
	DeleteCachedWorktree                   bool
	IgnoredFiles                           *IgnoreFileFlag
	BeforeQueryErrorBehavior               *string
	TargetsFlag                            *string
	AnalysisCacheClearStrategy             *string
	CompareQueriesAroundAnalysisCacheClear bool
}

func StrPtr() *string {
	var s string
	return &s
}

func RegisterCommonFlags() *CommonFlags {
	commonFlags := CommonFlags{
		WorkingDirectory:                       StrPtr(),
		BazelPath:                              StrPtr(),
		BazelStartupOpts:                       &MultipleStrings{},
		BazelOpts:                              &MultipleStrings{},
		EnforceCleanRepo:                       AllowIgnored,
		DeleteCachedWorktree:                   false,
		IgnoredFiles:                           &IgnoreFileFlag{},
		BeforeQueryErrorBehavior:               StrPtr(),
		TargetsFlag:                            StrPtr(),
		AnalysisCacheClearStrategy:             StrPtr(),
		CompareQueriesAroundAnalysisCacheClear: false,
	}
	flag.StringVar(commonFlags.WorkingDirectory, "working-directory", ".", "Working directory to query.")
	flag.StringVar(commonFlags.BazelPath, "bazel", "bazel",
		"Bazel binary (basename on $PATH, or absolute or relative path) to run.")
	flag.Var(commonFlags.BazelStartupOpts, "bazel-startup-opts", "Startup options to pass to Bazel.")
	flag.Var(commonFlags.BazelOpts, "bazel-opts", "Options to pass to Bazel. Assumed to apply to build and cquery.")
	flag.Var(&commonFlags.EnforceCleanRepo, "enforce-clean",
		fmt.Sprintf("Pass --enforce-clean=%v to fail if the repository is unclean, or --enforce-clean=%v to allow ignored untracked files (the default).",
			EnforceClean.String(), AllowIgnored.String()))
	flag.BoolVar(&commonFlags.DeleteCachedWorktree, "delete-cached-worktree", false,
		"Delete created worktrees after use when created. Keeping them can make subsequent invocations faster.")
	flag.Var(commonFlags.IgnoredFiles, "ignore-file",
		"Files to ignore for git operations, relative to the working-directory. These files shan't affect the Bazel graph.")
	flag.StringVar(commonFlags.BeforeQueryErrorBehavior, "before-query-error-behavior", "ignore-and-build-all", "How to behave if the 'before' revision query fails. Accepted values: fatal,ignore-and-build-all")
	flag.StringVar(commonFlags.TargetsFlag, "targets", "//...",
		"Targets to consider. Accepts any valid `bazel query` expression (see https://bazel.build/reference/query).")
	flag.StringVar(commonFlags.AnalysisCacheClearStrategy, "analysis-cache-clear-strategy", "skip", "Strategy for clearing the analysis cache. Accepted values: skip,shutdown,discard.")
	flag.BoolVar(&commonFlags.CompareQueriesAroundAnalysisCacheClear, "compare-queries-around-analysis-cache-clear", false, "Whether to check for query result differences before and after analysis cache clears. This is a temporary flag for performing real-world analysis.")
	return &commonFlags
}

type CommonConfig struct {
	Context        *pkg.Context
	RevisionBefore pkg.LabelledGitRev
	Targets        pkg.TargetsList
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

	bazelCmd := pkg.DefaultBazelCmd{
		BazelPath:        *commonFlags.BazelPath,
		BazelStartupOpts: *commonFlags.BazelStartupOpts,
		BazelOpts:        *commonFlags.BazelOpts,
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
		DeleteCachedWorktree:                   commonFlags.DeleteCachedWorktree,
		IgnoredFiles:                           *commonFlags.IgnoredFiles,
		BeforeQueryErrorBehavior:               *commonFlags.BeforeQueryErrorBehavior,
		AnalysisCacheClearStrategy:             *commonFlags.AnalysisCacheClearStrategy,
		CompareQueriesAroundAnalysisCacheClear: commonFlags.CompareQueriesAroundAnalysisCacheClear,
	}

	// Non-context attributes

	beforeRev, err := pkg.NewLabelledGitRev(workingDirectory, beforeRevStr, "before")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve the \"before\" git revision: %w", err)
	}

	targetsList, err := pkg.ParseTargetsList(*commonFlags.TargetsFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to parse targets: %w", err)
	}

	// Additional checks

	uncleanFileStatuses, err := pkg.GitStatusFiltered(workingDirectory, *commonFlags.IgnoredFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to check whether the repository is clean: %w", err)
	}

	if len(uncleanFileStatuses) > 0 && commonFlags.EnforceCleanRepo == EnforceClean {
		log.Printf("Current working tree has %v non-ignored untracked files:\n",
			len(uncleanFileStatuses))
		for _, status := range uncleanFileStatuses {
			log.Printf("%s\n", status)
		}
		return nil, fmt.Errorf("current repository is not clean and --enforce-clean option is set to '%v'. Exiting.", EnforceClean.String())
	}

	return &CommonConfig{
		Context:        context,
		RevisionBefore: beforeRev,
		Targets:        targetsList,
	}, nil
}

type MultipleStrings []string

func (s *MultipleStrings) String() string {
	return fmt.Sprintf("%v", MultipleStrings{})
}

func (s *MultipleStrings) Set(value string) error {
	*s = append(*s, value)
	return nil
}
