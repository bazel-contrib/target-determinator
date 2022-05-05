package pkg

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/aristanetworks/goarista/path"
	"github.com/bazel-contrib/target-determinator/common"
	ss "github.com/bazel-contrib/target-determinator/common/sorted_set"
	"github.com/bazel-contrib/target-determinator/third_party/protobuf/bazel/analysis"
	build "github.com/bazel-contrib/target-determinator/third_party/protobuf/bazel/build"
	"github.com/bazelbuild/bazel-gazelle/label"
	"google.golang.org/protobuf/proto"
)

type Configuration string

type LabelledGitRev struct {
	// Revision represents the git sha or ref. These values must be absolute.
	// A value such as "HEAD^" first needs to be resolved to the relevant commit.
	Revision string
	// Label is a description of what the git sha represents which may be useful to humans.
	Label string
	// Sha is the resolved sha256 of the Revision.
	Sha string
}

// NewLabelledGitRev ensures that the git sha is resolved as soon as the object is created, otherwise we might encounter
// undesirable behaviors when switching to other revisions e.g. if using "HEAD".
func NewLabelledGitRev(workspacePath string, revision string, label string) (LabelledGitRev, error) {
	lgr := LabelledGitRev{Revision: revision, Label: label, Sha: ""}
	sha, err := GitRevParse(workspacePath, revision, false)
	if err != nil {
		return lgr, fmt.Errorf("failed to resolve revision %v: %w", revision, err)
	}
	lgr.Sha = sha

	// If the provided revision is not a symbolic ref such as a branch then it might be relative to
	// the current HEAD (e.g. "HEAD" or "HEAD^"), in which case we resolve the SHA to make it absolute.
	symbolicRef, err := GitRevParse(workspacePath, revision, true)
	if err != nil {
		return lgr, fmt.Errorf("failed to resolve sybolic ref for revision %v: %w", revision, err)
	}
	if symbolicRef == "" || symbolicRef == "HEAD" {
		lgr.Revision = sha
	}

	return lgr, nil
}

func (l LabelledGitRev) String() string {
	s := fmt.Sprintf("%s (", l.Label)
	if l.Revision != l.Sha {
		s += "revision: "
		s += l.Revision
		s += ", "
	}
	s += "sha: " + l.Sha + ")"
	return s
}

type Context struct {
	// OriginalWorkspacePath is the absolute path to the root of the project's Bazel Workspace directory (which is
	// assumed to be in a git repository, but is not assumed to be the root of a git repository).
	OriginalWorkspacePath string
	// OriginalRevision is the git revision the repo was in when initializing the context.
	OriginalRevision LabelledGitRev
	// BazelPath is the path (or basename to be looked up in $PATH) of the Bazel to invoke.
	BazelPath string
	// CurrentWorkspacePath is the absolute path to the root of the Bazel Workspace directory we are currently
	// processing. It may be different from OriginalWorkspacePath at some point if creation of a git worktree is needed.
	CurrentWorkspacePath string
	// IgnoredFiles represents files that should be ignored for git operations.
	IgnoredFiles []common.RelPath
}

// FullyProcess returns the before and after metadata maps, with fully filled caches.
func FullyProcess(context *Context, revBefore, revAfter LabelledGitRev, pattern label.Pattern) (*QueryResults, *QueryResults, error) {
	err := EnsurePreconditions(context)
	if err != nil {
		return nil, nil, err
	}

	// At this point, it's mostly safe to run `git clean` because we know that the git repo was clean in the first place.
	// We still need to check that we did end up back on the original commit.
	defer Cleanup(context)

	queryInfoBefore, newContext, err := fullyProcessRevision(context, revBefore, pattern)
	if err != nil {
		return nil, nil, err
	}

	queryInfoAfter, _, err := fullyProcessRevision(newContext, revAfter, pattern)
	if err != nil {
		return nil, nil, err
	}

	return queryInfoBefore, queryInfoAfter, nil
}

func fullyProcessRevision(context *Context, rev LabelledGitRev, pattern label.Pattern) (*QueryResults, *Context, error) {
	log.Printf("Checking out: %s", rev)
	queryInfo, newContext, loadMetadataCleanup, err := LoadIncompleteMetadata(context, rev, pattern)
	defer loadMetadataCleanup()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load metadata at %s: %w", rev, err)
	}

	log.Printf("Hashing targets for %s", rev)
	if err := queryInfo.PrefillCache(); err != nil {
		return nil, nil, fmt.Errorf("failed to calculate hashes at %s: %w", rev, err)
	}
	return queryInfo, newContext, nil
}

func EnsurePreconditions(context *Context) error {
	isClean, err := EnsureGitRepositoryClean(context.OriginalWorkspacePath, context.IgnoredFiles)
	if err != nil {
		return fmt.Errorf("failed to check whether the repository is clean: %w", err)
	}
	if !isClean {
		return fmt.Errorf("current git working copy is not clean")
	}
	return nil
}

func Cleanup(context *Context) {
	finalSha, err := GitRevParse(context.OriginalWorkspacePath, "HEAD", false)
	if err != nil {
		log.Printf("failed to parse 'HEAD' git revision: %v", err)
	}
	if finalSha != context.OriginalRevision.Sha {
		log.Printf("error: the final git sha (%v) does not match the original git sha (%v). Skipping git clean.",
			finalSha, context.OriginalRevision.Sha)
		return
	}
	log.Printf("Cleaning up")
	err = GitClean(context.OriginalWorkspacePath, context.IgnoredFiles)
	if err != nil {
		log.Printf("Warning: failed to run `git clean` on project workspace: %v.", err)
	}
}

// LoadIncompleteMetadata loads the metadata about, but not hashes of, targets into a QueryResults.
// The (transitive) dependencies of the passed pattern will be loaded. For all targets, pass the
// pattern `//...`.
//
// This function returns a non-nil callback to clean up the worktree if it was created.
func LoadIncompleteMetadata(context *Context, rev LabelledGitRev, pattern label.Pattern) (*QueryResults, *Context, func(), error) {
	newContext := Context{
		OriginalWorkspacePath: context.OriginalWorkspacePath,
		OriginalRevision:      context.OriginalRevision,
		BazelPath:             context.BazelPath,
		CurrentWorkspacePath:  context.OriginalWorkspacePath,
		IgnoredFiles:          context.IgnoredFiles,
	}
	cleanupFunc := func() {}

	// This may return a different workspace path to ensure we don't destroy any local data.
	newWorkspacePath, err2 := gitSafeCheckout(context.OriginalWorkspacePath, rev, context.IgnoredFiles)
	if err2 != nil {
		return nil, nil, cleanupFunc, fmt.Errorf("failed to checkout %s in %v: %w", rev, context.OriginalWorkspacePath, err2)
	}

	// A worktree was created by gitSafeCheckout(). Use it and set the cleanup callback.
	if newWorkspacePath != "" {
		cleanupFunc = func() {
			err := os.RemoveAll(newWorkspacePath)
			if err != nil {
				log.Printf("Warning: failed to clean up temporary git worktree at %s: %v.", newWorkspacePath, err)
			}
		}
		newContext.CurrentWorkspacePath = newWorkspacePath
	}

	// Clear analysis cache before each query, as cquery configurations leak across invocations.
	// See https://github.com/bazelbuild/bazel/issues/14725
	if err := clearAnalysisCache(&newContext); err != nil {
		return nil, nil, cleanupFunc, err
	}

	queryInfo, err := doQueryDeps(&newContext, pattern)
	if err != nil {
		return nil, nil, cleanupFunc, fmt.Errorf("failed to query at %s in %v: %w", rev, newWorkspacePath, err)
	}
	return queryInfo, &newContext, cleanupFunc, nil
}

// stringSliceContainsStartingWith returns whether slice contains items that are a path prefix of element.
func stringSliceContainsStartingWith(pathPrefixes []common.RelPath, element common.RelPath) bool {
	for _, s := range pathPrefixes {
		if path.HasPrefix(element.Path(), s.Path()) {
			return true
		}
	}
	return false
}

func EnsureGitRepositoryClean(workingDirectory string, ignoredFiles []common.RelPath) (bool, error) {
	uncleanFileStatuses, err := gitStatus(workingDirectory)
	if err != nil {
		return false, err
	}
	var filteredUncleanStatuses []GitFileStatus
	for _, status := range uncleanFileStatuses {
		if !stringSliceContainsStartingWith(ignoredFiles, status.FilePath) {
			filteredUncleanStatuses = append(filteredUncleanStatuses, status)
		}
	}
	if len(filteredUncleanStatuses) > 0 {
		log.Printf("Current working tree has %v non-ignored untracked files:\n",
			len(filteredUncleanStatuses))
		for _, status := range filteredUncleanStatuses {
			log.Printf("%s\n", status)
		}
		return false, nil
	}
	return true, nil
}

func GitRevParse(workingDirectory string, rev string, isAbbrevRef bool) (string, error) {
	gitArgs := []string{"rev-parse"}
	if isAbbrevRef {
		gitArgs = append(gitArgs, "--abbrev-ref")
	}
	gitArgs = append(gitArgs, rev)
	gitCmd := exec.Command("git", gitArgs...)
	gitCmd.Dir = workingDirectory
	var stdoutBuf, stderrBuf bytes.Buffer
	gitCmd.Stdout = &stdoutBuf
	gitCmd.Stderr = &stderrBuf
	err := gitCmd.Run()
	if err != nil {
		return "", fmt.Errorf("could not parse revision '%v': %w. Stderr: %v", rev, err, stderrBuf.String())
	}
	return strings.Trim(stdoutBuf.String(), "\n"), nil
}

type GitFileStatus struct {
	// Status contains the shorthand notation of the status of the file. See `man git-status` for a mapping.
	Status string
	// FilePath represents the path of the file relative to the git repository.
	FilePath common.RelPath
}

func (s GitFileStatus) String() string {
	return fmt.Sprintf("%3v %v", s.Status, s.FilePath.String())
}

func gitStatus(workingDirectory string) ([]GitFileStatus, error) {
	dirtyFileStatuses, err := runToLines(workingDirectory, "git", "status", "--porcelain", "--ignore-submodules=none")
	if err != nil {
		return nil, fmt.Errorf("failed to get git status: %w", err)
	}
	var gitFileStatuses []GitFileStatus
	for _, status := range dirtyFileStatuses {
		gitFileStatuses = append(gitFileStatuses, GitFileStatus{
			Status:   strings.TrimSpace(status[0:3]),
			FilePath: common.NewRelPath(strings.TrimSpace(status[3:])),
		})
	}
	return gitFileStatuses, nil
}

// gitSafeCheckout checks out a sha and its recursive submodules in a repository.
// If there are any untracked files after the checkout, a worktree is created to avoid deleting user's files and the
// function returns the path to the new worktree, otherwise nil is returned.
//
// The caller has the responsibility to clean up the worktree.
//
// Notes:
// - even if the repository is clean before the checkout, there is still a possibility that file `foo` is
// ignored in the original commit but not in the target commit (e.g. if it was recently added to the `.gitignore`).
// - the repository might also be unclean after a checkout if a submodule was moved or removed between the current and
// target commit.
func gitSafeCheckout(workingDirectory string, rev LabelledGitRev, ignoredFiles []common.RelPath) (string, error) {
	gitCmd := exec.Command("git", "checkout", rev.Revision)
	gitCmd.Dir = workingDirectory
	if output, err := gitCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to check out %s: %w. Output: %v", rev, err, string(output))
	}

	newRepositoryPath := ""
	isClean, err := EnsureGitRepositoryClean(workingDirectory, ignoredFiles)
	if err != nil {
		return "", fmt.Errorf("failed to check the repository is clean: %w", err)
	}
	if !isClean {
		tempDir, err := os.MkdirTemp("", "td-worktree-*")
		if err != nil {
			return "", fmt.Errorf("failed to create temporary directory for git worktree: %w", err)
		}

		if err = gitCreateWorktree(workingDirectory, tempDir); err != nil {
			return "", fmt.Errorf("failed to create temporary git worktree: %w", err)
		}
		workingDirectory = tempDir
		newRepositoryPath = tempDir
	}

	gitCmd = exec.Command("git", "submodule", "update", "--init", "--recursive")
	gitCmd.Dir = workingDirectory
	if output, err := gitCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to update submodules during checkout %s: %w. Output: %v", rev, err, string(output))
	}
	return newRepositoryPath, nil
}

// Create a detached worktree in targetDirectory from the repo present in workingDirectory.
func gitCreateWorktree(workingDirectory string, targetDirectory string) error {
	gitCmd := exec.Command("git", "worktree", "add", "--detach", targetDirectory)
	gitCmd.Dir = workingDirectory
	if output, err := gitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add temporary git worktree: %w. Output: %v", err, string(output))
	}
	return nil
}

// GitClean cleans the repository. This does not include ignored files.
func GitClean(workingDirectory string, ignoredFiles []common.RelPath) error {
	args := []string{"clean", "-ffd"}
	for _, ignoredFile := range ignoredFiles {
		args = append(args, "--exclude", fmt.Sprintf("/%s", ignoredFile.String()))
	}
	gitCmd := exec.Command("git", args...)

	gitCmd.Dir = workingDirectory
	if output, err := gitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to clean git repository: %w. Output: %v", err, string(output))
	}
	return nil
}

type QueryResults struct {
	MatchingTargets             *MatchingTargets
	TransitiveConfiguredTargets map[label.Label]map[Configuration]*analysis.ConfiguredTarget
	TargetHashCache             *TargetHashCache
}

func (queryInfo *QueryResults) PrefillCache() error {
	var err error
	var numWorkers int
	workerCountEnv := os.Getenv("TD_WORKER_COUNT")
	if workerCountEnv == "" {
		numWorkers = runtime.NumCPU() * 8
	} else {
		numWorkers, err = strconv.Atoi(workerCountEnv)
		if err != nil {
			return fmt.Errorf("could not parse the TD_WORKER_COUNT env var into an int: %v", workerCountEnv)
		}
	}

	// Create a thread pool to hash the targets faster.
	labelAndConfigurationsChan := make(chan LabelAndConfiguration, numWorkers)
	errorsChan := make(chan error, 1)

	var once sync.Once
	var wg sync.WaitGroup
	for w := 1; w <= numWorkers; w++ {
		go func() {
			for labelAndConfiguration := range labelAndConfigurationsChan {
				_, err := queryInfo.TargetHashCache.Hash(labelAndConfiguration)
				if err != nil {
					once.Do(func() { errorsChan <- err }) // We only return one error.
				}
				wg.Done()
			}
		}()
	}

OUTER:
	for _, label := range queryInfo.MatchingTargets.Labels() {
		for _, configuration := range queryInfo.MatchingTargets.ConfigurationsFor(label) {
			if len(errorsChan) > 0 {
				break OUTER
			}
			wg.Add(1)
			labelAndConfigurationsChan <- LabelAndConfiguration{
				Label:         label,
				Configuration: configuration,
			}
		}
	}

	// Signal to workers that work is done.
	close(labelAndConfigurationsChan)
	wg.Wait()

	if len(errorsChan) > 0 {
		return <-errorsChan
	}

	// We may be about to change the filesystem state, which will mean any file reads done after
	// this point may be invalid.
	// We freeze the TargetHashCache to ensure it will not allow further reads after this point.
	queryInfo.TargetHashCache.Freeze()
	return nil
}

type LabelAndConfigurations struct {
	Label          label.Label
	Configurations []Configuration
}

type LabelAndConfiguration struct {
	Label         label.Label
	Configuration Configuration
}

var targetConfigRegexp = regexp.MustCompile("^([^ ]+) \\(([0-9a-fA-Z]*)\\)$")

func clearAnalysisCache(context *Context) error {
	// Discard the analysis cache:
	{
		var stderr bytes.Buffer
		cmd := exec.Command(context.BazelPath, "build", "--discard_analysis_cache")
		cmd.Dir = context.CurrentWorkspacePath
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to discard Bazel analysis cache in %v: %w. Stderr:\n%v", context.CurrentWorkspacePath, err, stderr.String())
		}
	}

	// --discard_analysis_cache defers some of its cleanup to the start of the next build.
	// Perform a no-op build to flush any in-build state from the previous one.
	{
		var stderr bytes.Buffer
		cmd := exec.Command(context.BazelPath, "build")
		cmd.Dir = context.CurrentWorkspacePath
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to run no-op build after discarding Bazel analysis cache in %v: %w. Stderr:\n%v", context.CurrentWorkspacePath, err, stderr.String())
		}
	}
	return nil
}

func doQueryDeps(context *Context, pattern label.Pattern) (*QueryResults, error) {
	depsPattern := fmt.Sprintf("deps(%s)", pattern.String())
	transitiveResult, err := runToCqueryResult(context, depsPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to cquery %v: %w", depsPattern, err)
	}

	transitiveConfiguredTargets, err := ParseCqueryResult(transitiveResult)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cquery result: %w", err)
	}

	matchingTargetResults, err := runToCqueryResult(context, pattern.String())
	if err != nil {
		return nil, fmt.Errorf("failed to run top-level cquery: %w", err)
	}

	log.Println("Matching labels to configurations")
	labels := make([]label.Label, 0)
	labelsToConfigurations := make(map[label.Label][]Configuration)
	for _, mt := range matchingTargetResults.Results {
		label, err := labelOf(mt.Target)
		if err != nil {
			return nil, fmt.Errorf("failed to parse label returned from query %s: %w", mt.Target, err)
		}
		labels = append(labels, label)

		configuration := mt.Configuration.Checksum
		labelsToConfigurations[label] = append(labelsToConfigurations[label], Configuration(configuration))
	}

	processedLabelsToConfigurations := make(map[label.Label]*ss.SortedSet[Configuration], len(labels))
	for label, configurations := range labelsToConfigurations {
		processedLabelsToConfigurations[label] = ss.NewSortedSet(configurations)
	}

	matchingTargets := &MatchingTargets{
		labels:                 ss.NewSortedSetFunc(labels, CompareLabels),
		labelsToConfigurations: processedLabelsToConfigurations,
	}

	queryResults := &QueryResults{
		MatchingTargets:             matchingTargets,
		TransitiveConfiguredTargets: transitiveConfiguredTargets,
		TargetHashCache:             NewTargetHashCache(transitiveConfiguredTargets),
	}
	return queryResults, nil
}

func runToCqueryResult(context *Context, pattern string) (*analysis.CqueryResult, error) {
	log.Printf("Running cquery on %s", pattern)
	var output bytes.Buffer
	var stderr bytes.Buffer
	queryCmd := exec.Command(context.BazelPath, "cquery", "--output=proto", pattern)
	queryCmd.Dir = context.CurrentWorkspacePath
	queryCmd.Stdout = &output
	queryCmd.Stderr = &stderr
	if err := queryCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to run cquery on %s: %w. Stderr:\n%v", pattern, err, stderr.String())
	}

	content := output.Bytes()

	var result analysis.CqueryResult
	if err := proto.Unmarshal(content, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cquery output: %w", err)
	}
	return &result, nil
}

// MatchingTargets stores the top-level targets within a repository,
// i.e. those matching the Bazel pattern `...`.
type MatchingTargets struct {
	labels                 *ss.SortedSet[label.Label]
	labelsToConfigurations map[label.Label]*ss.SortedSet[Configuration]
}

func (mt *MatchingTargets) Labels() []label.Label {
	return mt.labels.SortedSlice()
}

func (mt *MatchingTargets) ConfigurationsFor(label label.Label) []Configuration {
	return mt.labelsToConfigurations[label].SortedSlice()
}

func (mt *MatchingTargets) ContainsLabelAndConfiguration(label label.Label, configuration Configuration) bool {
	configurations, ok := mt.labelsToConfigurations[label]
	if !ok {
		return false
	}
	return configurations.Contains(configuration)
}

func runToLines(workingDirectory string, arg0 string, args ...string) ([]string, error) {
	cmd := exec.Command(arg0, args...)
	cmd.Dir = workingDirectory
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("%w. Stderr: %v", err, stderrBuf.String())
	}
	return strings.FieldsFunc(stdoutBuf.String(), func(r rune) bool { return r == '\n' }), nil
}

func ParseCqueryResult(result *analysis.CqueryResult) (map[label.Label]map[Configuration]*analysis.ConfiguredTarget, error) {
	configuredTargets := make(map[label.Label]map[Configuration]*analysis.ConfiguredTarget, len(result.Results))

	for _, target := range result.Results {
		label, err := labelOf(target.GetTarget())
		if err != nil {
			return nil, err
		}

		_, ok := configuredTargets[label]
		if !ok {
			configuredTargets[label] = make(map[Configuration]*analysis.ConfiguredTarget)
		}

		configuredTargets[label][Configuration(target.GetConfiguration().GetChecksum())] = target
	}
	return configuredTargets, nil
}

func labelOf(target *build.Target) (label.Label, error) {
	switch target.GetType() {
	case build.Target_RULE:
		return label.Parse(target.GetRule().GetName())
	case build.Target_SOURCE_FILE:
		return label.Parse(target.GetSourceFile().GetName())
	case build.Target_GENERATED_FILE:
		return label.Parse(target.GetGeneratedFile().GetName())
	case build.Target_PACKAGE_GROUP:
		return label.Parse(target.GetPackageGroup().GetName())
	case build.Target_ENVIRONMENT_GROUP:
		return label.Parse(target.GetEnvironmentGroup().GetName())
	default:
		return label.NoLabel, fmt.Errorf("labelOf called on unknown target type: %v", target.GetType().String())
	}
	return label.NoLabel, nil
}

func equivalentAttributes(left, right *build.Attribute) bool {
	return proto.Equal(AttributeForSerialization(left), AttributeForSerialization(right))
}

// AttributeForSerialization redacts details about an attribute which don't affect the output of
// building them, and returns equivalent canonical attribute metadata.
// In particular it redacts:
//  * Whether an attribute was explicitly specified (because the effective value is all that
//    matters).
//  * Any attribute named `generator_location`, because these point to absolute paths for
//    built-in `cc_toolchain_suite` targets such as `@local_config_cc//:toolchain`.
func AttributeForSerialization(rawAttr *build.Attribute) *build.Attribute {
	normalized := *rawAttr
	normalized.ExplicitlySpecified = nil

	// Redact generator_location, which typically contains absolute paths but has no bearing on the
	// functioning of a rule.
	// This is also done in Bazel's internal target hash computation. See:
	// https://github.com/bazelbuild/bazel/blob/6971b016f1e258e3bb567a0f9fe7a88ad565d8f2/src/main/java/com/google/devtools/build/lib/query2/query/output/SyntheticAttributeHashCalculator.java#L78-L81
	if normalized.Name != nil {
		if *normalized.Name == "generator_location" {
			normalized.StringValue = nil
		}
	}

	return &normalized
}

func CompareLabels(a, b label.Label) bool {
	return a.String() < b.String()
}
