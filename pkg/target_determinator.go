package pkg

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	path2 "path"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/aristanetworks/goarista/path"
	"github.com/bazel-contrib/target-determinator/common"
	ss "github.com/bazel-contrib/target-determinator/common/sorted_set"
	"github.com/bazel-contrib/target-determinator/third_party/protobuf/bazel/analysis"
	"github.com/bazel-contrib/target-determinator/third_party/protobuf/bazel/build"
	"github.com/bazelbuild/bazel-gazelle/label"
	"google.golang.org/protobuf/proto"
)

type Configuration string

type LabelledGitRev struct {
	// Label is a description of what the git sha represents which may be useful to humans.
	Label string
	// GitRev is the actual revision.
	GitRevision GitRev
}

type GitRev struct {
	// Revision represents the git sha or ref. These values must be absolute.
	// A value such as "HEAD^" first needs to be resolved to the relevant commit.
	Revision string
	// Sha is the resolved sha256 of the Revision.
	Sha string
}

// NoLabelledGitRev represents a null value for LabelledGitRev.
var NoLabelledGitRev LabelledGitRev

// CurrentWorkingDirState represents the (potentially dirty) state of the current working directory.
var CurrentWorkingDirState GitRev

// NewLabelledGitRev ensures that the git sha is resolved as soon as the object is created, otherwise we might encounter
// undesirable behaviors when switching to other revisions e.g. if using "HEAD".
// If the revision argument is empty, the returned object will return the current workspace's (potentially dirty) state.
func NewLabelledGitRev(workspacePath string, revision string, label string) (LabelledGitRev, error) {
	var gr GitRev
	if revision == "" {
		gr = CurrentWorkingDirState
	} else {
		gr = GitRev{Revision: revision, Sha: ""}
		sha, err := GitRevParse(workspacePath, revision, false)
		if err != nil {
			return NoLabelledGitRev, fmt.Errorf("failed to resolve revision %v: %w", revision, err)
		}
		gr.Sha = sha

		// If the provided revision is not a symbolic ref such as a branch then it might be relative to
		// the current HEAD (e.g. "HEAD" or "HEAD^"), in which case we resolve the SHA to make it absolute.
		symbolicRef, err := GitRevParse(workspacePath, revision, true)
		if err != nil {
			return NoLabelledGitRev, fmt.Errorf("failed to resolve sybolic ref for revision %v: %w", revision, err)
		}
		if symbolicRef == "" || symbolicRef == "HEAD" {
			gr.Revision = sha
		}
	}

	return LabelledGitRev{Label: label, GitRevision: gr}, nil
}

func (l LabelledGitRev) String() string {
	return fmt.Sprintf("revision '%s' (%s)", l.Label, l.GitRevision)
}

func (l GitRev) String() string {
	s := ""
	if l == CurrentWorkingDirState {
		s = "current working directory state"
	} else {
		if l.Revision != l.Sha {
			s += l.Revision
			s += ", "
		}
		s += "sha: " + l.Sha
	}
	return s
}

type Context struct {
	// WorkspacePath is the absolute path to the root of the project's Bazel Workspace directory (which is
	// assumed to be in a git repository, but is not assumed to be the root of a git repository).
	WorkspacePath string
	// OriginalRevision is the git revision the repo was in when initializing the context.
	OriginalRevision LabelledGitRev
	// BazelCmd is used to execute when necessary Bazel.
	BazelCmd BazelCmd
	// BazelOutputBase is the path of the Bazel output base directory of the original workspace.
	BazelOutputBase string
	// DeleteCachedWorktree represents whether we should keep worktrees around for reuse in future invocations.
	DeleteCachedWorktree bool
	// IgnoredFiles represents files that should be ignored for git operations.
	IgnoredFiles []common.RelPath
	// AnalysisCacheClearStrategy is the strategy used for clearing the Bazel analysis cache before cquery runs.
	// Accepted values are: skip, shutdown, discard.
	// We currently don't believe clearing this cache is necessary.
	//
	// skip will not clear the analysis cache between cquery runs.
	//
	// shutdown will shut down the bazel server before queries.
	// discard will run a build with --discard_analysis_cache before queries.
	//
	// discard avoids a potentially costly JVM tear-down and start-up,
	/// but seems to over-invalidate things (e.g. it seems to force re-fetching every rules_python whl_library which can be very expensive).
	AnalysisCacheClearStrategy string
	// CompareQueriesAroundAnalysisCacheClear controls whether we validate whether clearing the analysis cache had any meaningful effect.
	// We suspect that clearing the analysis cache is now unnecessary, as cquery behaves more reasonably around not returning stale results.
	// This flag allows validating whether that is the case.
	CompareQueriesAroundAnalysisCacheClear bool
}

// FullyProcess returns the before and after metadata maps, with fully filled caches.
func FullyProcess(context *Context, revBefore LabelledGitRev, revAfter LabelledGitRev, targets TargetsList) (*QueryResults, *QueryResults, error) {
	log.Printf("Processing %s", revBefore)
	queryInfoBefore, err := fullyProcessRevision(context, revBefore, targets)
	if err != nil {
		if queryInfoBefore == nil {
			return nil, nil, err
		} else {
			log.Printf("A query error occurred querying %s - ignoring the error and treating all matching targets from the '%s' revision as affected. Error querying: %v", revBefore, revAfter.Label, err)
		}
	}

	// At this point, we assume that the working directory is back to its pristine state.
	log.Printf("Processing %s", revAfter)
	queryInfoAfter, err := fullyProcessRevision(context, revAfter, targets)
	if err != nil {
		return nil, nil, err
	}

	return queryInfoBefore, queryInfoAfter, nil
}

// fullyProcessRevision may return a nil error and a non-nil queryInfo.
// This indicates that evaluating the initial query at this revision failed,
// but that the user may want to use the results anyway, despite their query results being empty.
// This may be useful when the "before" commit is broken for query, as it allows for running all
// matching targets from the "after" query, despite the "before" being broken.
func fullyProcessRevision(context *Context, rev LabelledGitRev, targets TargetsList) (queryInfo *QueryResults, err error) {
	defer func() {
		innerErr := gitCheckout(context.WorkspacePath, context.OriginalRevision)
		if innerErr != nil && err == nil {
			err = fmt.Errorf("failed to check out original commit during cleanup: %v", err)
		}
	}()
	queryInfo, loadMetadataCleanup, err := LoadIncompleteMetadata(context, rev, targets)
	defer loadMetadataCleanup()
	if err != nil {
		return queryInfo, fmt.Errorf("failed to load metadata at %s: %w", rev, err)
	}

	log.Println("Hashing targets")
	if err := queryInfo.PrefillCache(); err != nil {
		return nil, fmt.Errorf("failed to calculate hashes at %s: %w", rev, err)
	}
	return queryInfo, nil
}

// LoadIncompleteMetadata loads the metadata about, but not hashes of, targets into a QueryResults.
// The (transitive) dependencies of the passed targets will be loaded. For all targets, use `//...`.
//
// It may change the git revision of the workspace to rev, in which case it is the caller's
// responsibility to check out the original commit.
//
// It returns a non-nil callback to clean up the worktree if it was created.
//
// Note that a non-nil QueryResults may be returned even in the error case, which will have an
// empty target-set, but may contain other useful information (e.g. the bazel release version).
// Checking for nil-ness of the error is the true arbiter for whether the entire load was successful.
func LoadIncompleteMetadata(context *Context, rev LabelledGitRev, targets TargetsList) (*QueryResults, func(), error) {
	// Create a temporary context to allow the workspace path to point to a git worktree if necessary.
	context = &Context{
		WorkspacePath:                          context.WorkspacePath,
		OriginalRevision:                       context.OriginalRevision,
		BazelCmd:                               context.BazelCmd,
		BazelOutputBase:                        context.BazelOutputBase,
		DeleteCachedWorktree:                   context.DeleteCachedWorktree,
		IgnoredFiles:                           context.IgnoredFiles,
		AnalysisCacheClearStrategy:             context.AnalysisCacheClearStrategy,
		CompareQueriesAroundAnalysisCacheClear: context.CompareQueriesAroundAnalysisCacheClear,
	}
	cleanupFunc := func() {}

	if rev.GitRevision != CurrentWorkingDirState {
		// This may return a new workspace path to ensure we don't destroy any local data.
		newWorkspacePath, err2 := gitSafeCheckout(context, rev, context.IgnoredFiles)

		// A worktree was created by gitSafeCheckout(). Use it and set the cleanup callback even
		// if gitSafeCheckout returns an error.
		if newWorkspacePath != "" && context.DeleteCachedWorktree {
			cleanupFunc = func() {
				err := os.RemoveAll(newWorkspacePath)
				if err != nil {
					err = fmt.Errorf("failed to clean up temporary git worktree at %s: %v", newWorkspacePath, err)
				}
			}
			context.WorkspacePath = newWorkspacePath
		}

		if err2 != nil {
			return nil, cleanupFunc, fmt.Errorf("failed to checkout %s in %v: %w", rev, context.WorkspacePath, err2)
		}
	}

	var queryInfoBeforeClear *QueryResults
	if context.CompareQueriesAroundAnalysisCacheClear {
		var err error
		queryInfoBeforeClear, err = doQueryDeps(context, targets)
		if err != nil {
			return queryInfoBeforeClear, cleanupFunc, fmt.Errorf("failed to query[before] at %s in %v: %w", rev, context.WorkspacePath, err)
		}
	}

	// Clear analysis cache before each query, as cquery configurations leak across invocations.
	// See https://github.com/bazelbuild/bazel/issues/14725
	if err := clearAnalysisCache(context); err != nil {
		return nil, cleanupFunc, err
	}

	queryInfo, err := doQueryDeps(context, targets)
	if err != nil {
		return queryInfo, cleanupFunc, fmt.Errorf("failed to query at %s in %v: %w", rev, context.WorkspacePath, err)
	}

	if context.CompareQueriesAroundAnalysisCacheClear {
		if !reflect.DeepEqual(queryInfoBeforeClear.MatchingTargets, queryInfo.MatchingTargets) {
			return nil, cleanupFunc, fmt.Errorf("inconsistent cquery results before and after analysis cache clear: MatchingTargets")
		}
		if !reflect.DeepEqual(queryInfoBeforeClear.TransitiveConfiguredTargets, queryInfo.TransitiveConfiguredTargets) {
			return nil, cleanupFunc, fmt.Errorf("inconsistent cquery results before and after analysis cache clear: TransitiveConfiguredTargets")
		}
	}

	return queryInfo, cleanupFunc, nil
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
	filteredUncleanStatuses, err := GitStatusFiltered(workingDirectory, ignoredFiles)
	if err != nil {
		return false, err
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
		return "", fmt.Errorf("could not parse revision '%v': %w. Stderr from git ↓↓\n%v", rev, err, stderrBuf.String())
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

func GitStatusFiltered(workingDirectory string, ignoredFiles []common.RelPath) ([]GitFileStatus, error) {
	uncleanFileStatuses, err := gitStatus(workingDirectory)
	if err != nil {
		return nil, err
	}
	var filteredUncleanStatuses []GitFileStatus
	for _, status := range uncleanFileStatuses {
		if !stringSliceContainsStartingWith(ignoredFiles, status.FilePath) {
			filteredUncleanStatuses = append(filteredUncleanStatuses, status)
		}
	}
	return filteredUncleanStatuses, nil
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
//
// Sometimes a worktree is created to avoid altering the repository, in which case the
// function returns the path to the new worktree, otherwise an empty string is returned.
//
// A new worktree is created in the following cases:
// - the original worktree is unclean (non-ignored untracked files or tracked local changes).
// - upon checking out the new revision, the worktree is unclean. This can happen when a submodule
//   was moved or removed between the current and target commit, or when the contents of the
//  .gitignore file changes.
//
// When a worktree is created, the repository present in workingDirectory may or may not have
// the rev revision checked out.
//
// When applicable, the caller is responsible for cleaning up the newly created worktree.
func gitSafeCheckout(context *Context, rev LabelledGitRev, ignoredFiles []common.RelPath) (string, error) {
	useGitWorktree := false
	isPreCheckoutClean, err := EnsureGitRepositoryClean(context.WorkspacePath, ignoredFiles)
	if err != nil {
		return "", fmt.Errorf("failed to check whether the repository is clean: %w", err)
	}
	if !isPreCheckoutClean {
		log.Printf("Workspace is unclean, using git worktree. This will be slower the first time. " +
			"You can avoid this by committing local changes and ignoring untracked files.")
		useGitWorktree = true
	} else {
		if err := gitCheckout(context.WorkspacePath, rev); err != nil {
			return "", err
		}

		isPostCheckoutClean, err := EnsureGitRepositoryClean(context.WorkspacePath, ignoredFiles)
		if err != nil {
			return "", fmt.Errorf("failed to check whether the repository is clean: %w", err)
		}
		if !isPostCheckoutClean {
			log.Printf("Detected unclean repository after checkout (likely due to submodule or " +
				".gitignore changes). Using git worktree to leave original repository pristine.")
			useGitWorktree = true
		}
	}
	newRepositoryPath := ""
	if useGitWorktree {
		newRepositoryPath, err = gitReuseOrCreateWorktree(context.WorkspacePath, rev)
		if err != nil {
			return "", fmt.Errorf("failed to create or reuse worktree: %w", err)
		}
		context.WorkspacePath = newRepositoryPath
	}

	gitCmd := exec.Command("git", "submodule", "update", "--init", "--recursive")
	gitCmd.Dir = context.WorkspacePath
	if output, err := gitCmd.CombinedOutput(); err != nil {
		return newRepositoryPath, fmt.Errorf("failed to update submodules during checkout %s: %w. Output: %v", rev, err, string(output))
	}
	return newRepositoryPath, nil
}

func gitCheckout(workingDirectory string, rev LabelledGitRev) error {
	gitCmd := exec.Command("git", "checkout", rev.GitRevision.Revision)
	gitCmd.Dir = workingDirectory
	if output, err := gitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to check out %s: %w. Output: %v", rev, err, string(output))
	}
	return nil
}

// gitReuseOrCreateWorktree tries to reuse an existing worktree from a previous invocation and check out the given revision.
// If it can't, it removes the directory completely and re-creates the worktree.
//
// The return path to the worktree is stable between invocations.
func gitReuseOrCreateWorktree(workingDirectory string, rev LabelledGitRev) (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to determine current user: %w", err)
	}
	cacheDir := path2.Join(currentUser.HomeDir, ".cache", "target-determinator")
	if err = os.MkdirAll(cacheDir, 0750); err != nil {
		return "", fmt.Errorf("failed to create the .cache directory")
	}
	hashBuilder := sha1.New()
	hashBuilder.Write([]byte(workingDirectory))
	currentDirHash := hex.EncodeToString(hashBuilder.Sum(nil))
	worktreeDirPath := path2.Join(cacheDir, fmt.Sprintf("td-worktree-%v-%v", path2.Base(workingDirectory), currentDirHash))

	if err := os.MkdirAll(cacheDir, 0750); err != nil {
		return "", fmt.Errorf("failed to create cache directory %v for git worktree: %w", worktreeDirPath, err)
	}

	tryReuseDir := true
	_, err = os.Stat(worktreeDirPath)
	if err != nil {
		if os.IsNotExist(err) {
			tryReuseDir = false
		}
	}

	// Attempt to git clean and check out the right revision, upon failure, nuke the directory and create a new worktree.
	if tryReuseDir {
		err := gitCleanCheckout(worktreeDirPath, rev.GitRevision.Sha)
		if err != nil {
			log.Printf("failed to reuse existing git worktree in %v: %v. Will re-create worktree.", worktreeDirPath, err)
		} else {
			// If we don't have any errors, our job is done.
			log.Printf("Reusing git worktree in %v", worktreeDirPath)
			return worktreeDirPath, nil
		}
	}

	err = os.RemoveAll(worktreeDirPath)
	if err != nil {
		return "", fmt.Errorf("failed to remove worktree directory %v: %w", worktreeDirPath, err)
	}
	if err = gitCreateWorktree(workingDirectory, worktreeDirPath, rev.GitRevision.Sha); err != nil {
		return worktreeDirPath, fmt.Errorf("failed to create temporary git worktree: %w", err)
	}

	log.Printf("Using fresh git worktree in %v", worktreeDirPath)
	return worktreeDirPath, nil
}

// gitCleanCheckout checks out the given commit and cleans uncommitted changes and untracked files, including ignored ones.
func gitCleanCheckout(workingDirectory string, rev string) error {
	gitCmd := exec.Command("git", "checkout", "-f", rev)
	gitCmd.Dir = workingDirectory
	if output, err := gitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to checkout rev %v in git worktree: %w. Output: %v", rev, err, string(output))
	}

	// Clean the repo, including ignored files.
	gitCmd = exec.Command("git", "clean", "-ffdx", rev)
	gitCmd.Dir = workingDirectory
	if output, err := gitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to clean git worktree: %w. Output: %v", err, string(output))
	}
	return nil
}

// Create a detached worktree in targetDirectory from the repo present in workingDirectory.
func gitCreateWorktree(workingDirectory string, targetDirectory string, rev string) error {
	gitCmd := exec.Command("git", "worktree", "add", "--force", "--force", "--detach", targetDirectory, rev)
	gitCmd.Dir = workingDirectory
	if output, err := gitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add temporary git worktree: %w. Output: %v", err, string(output))
	}
	return nil
}

type QueryResults struct {
	MatchingTargets             *MatchingTargets
	TransitiveConfiguredTargets map[label.Label]map[Configuration]*analysis.ConfiguredTarget
	TargetHashCache             *TargetHashCache
	BazelRelease                string
	// QueryError is whatever error was returned when running the cquery to get these results.
	QueryError     error
	configurations map[Configuration]singleConfigurationOutput
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
	for _, l := range queryInfo.MatchingTargets.Labels() {
		for _, configuration := range queryInfo.MatchingTargets.ConfigurationsFor(l) {
			if len(errorsChan) > 0 {
				break OUTER
			}
			wg.Add(1)
			labelAndConfigurationsChan <- LabelAndConfiguration{
				Label:         l,
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

func clearAnalysisCache(context *Context) error {
	if context.AnalysisCacheClearStrategy == "skip" {
		return nil
	} else if context.AnalysisCacheClearStrategy == "shutdown" {
		result, err := context.BazelCmd.Execute(BazelCmdConfig{Dir: context.WorkspacePath}, []string{"--output_base", context.BazelOutputBase}, "shutdown")
		if result != 0 || err != nil {
			return fmt.Errorf("failed to discard Bazel analysis cache in %v", context.WorkspacePath)
		}
		return nil
	} else if context.AnalysisCacheClearStrategy == "discard" {
		{
			var stderr bytes.Buffer

			result, err := context.BazelCmd.Execute(
				BazelCmdConfig{Dir: context.WorkspacePath, Stderr: &stderr},
				[]string{"--output_base", context.BazelOutputBase}, "build", "--discard_analysis_cache")

			if result != 0 || err != nil {
				return fmt.Errorf("failed to discard Bazel analysis cache in %v: %w. Stderr from Bazel ↓↓\n%v", context.WorkspacePath, err, stderr.String())
			}
		}

		// --discard_analysis_cache defers some of its cleanup to the start of the next build.
		// Perform a no-op build to flush any in-build state from the previous one.
		{
			var stderr bytes.Buffer

			result, err := context.BazelCmd.Execute(
				BazelCmdConfig{Dir: context.WorkspacePath, Stderr: &stderr},
				[]string{"--output_base", context.BazelOutputBase}, "build")

			if result != 0 || err != nil {
				return fmt.Errorf("failed to run no-op build after discarding Bazel analysis cache in %v: %w. Stderr:\n%v",
					context.WorkspacePath, err, stderr.String())
			}
		}
		return nil
	} else {
		return fmt.Errorf("unrecognized analysis cache discard strategy: %v", context.AnalysisCacheClearStrategy)
	}
}

func BazelOutputBase(workingDirectory string, BazelCmd BazelCmd) (string, error) {
	return bazelInfo(workingDirectory, BazelCmd, "output_base")
}

func BazelRelease(workingDirectory string, BazelCmd BazelCmd) (string, error) {
	return bazelInfo(workingDirectory, BazelCmd, "release")
}

func bazelInfo(workingDirectory string, bazelCmd BazelCmd, key string) (string, error) {
	var stdoutBuf, stderrBuf bytes.Buffer

	result, err := bazelCmd.Execute(
		BazelCmdConfig{Dir: workingDirectory, Stdout: &stdoutBuf, Stderr: &stderrBuf},
		nil, "info", key)

	if result != 0 || err != nil {
		return "", fmt.Errorf("failed to get the Bazel %v: %w. Stderr:\n%v", key, err, stderrBuf.String())
	}
	return strings.TrimRight(stdoutBuf.String(), "\n"), nil
}

// Note that a non-nil QueryResults may be returned even in the error case, which will have an
// empty target-set, but may contain other useful information (e.g. the bazel release version).
// Checking for nil-ness of the error is the true arbiter for whether the entire query was successful.
func doQueryDeps(context *Context, targets TargetsList) (*QueryResults, error) {
	bazelRelease, err := BazelRelease(context.WorkspacePath, context.BazelCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve the bazel release: %w", err)
	}

	depsPattern := fmt.Sprintf("deps(%s)", targets.String())
	transitiveResult, err := runToCqueryResult(context, depsPattern)
	if err != nil {
		retErr := fmt.Errorf("failed to cquery %v: %w", depsPattern, err)
		return &QueryResults{
			MatchingTargets: &MatchingTargets{
				labels:                 nil,
				labelsToConfigurations: nil,
			},
			TransitiveConfiguredTargets: nil,
			TargetHashCache:             NewTargetHashCache(nil, bazelRelease),
			BazelRelease:                bazelRelease,
			QueryError:                  retErr,
		}, retErr
	}

	transitiveConfiguredTargets, err := ParseCqueryResult(transitiveResult)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cquery result: %w", err)
	}

	matchingTargetResults, err := runToCqueryResult(context, targets.String())
	if err != nil {
		return nil, fmt.Errorf("failed to run top-level cquery: %w", err)
	}

	log.Println("Matching labels to configurations")
	labels := make([]label.Label, 0)
	labelsToConfigurations := make(map[label.Label][]Configuration)
	for _, mt := range matchingTargetResults.Results {
		l, err := labelOf(mt.Target)
		if err != nil {
			return nil, fmt.Errorf("failed to parse label returned from query %s: %w", mt.Target, err)
		}
		labels = append(labels, l)

		configuration := mt.Configuration.Checksum
		labelsToConfigurations[l] = append(labelsToConfigurations[l], Configuration(configuration))
	}

	processedLabelsToConfigurations := make(map[label.Label]*ss.SortedSet[Configuration], len(labels))
	for l, configurations := range labelsToConfigurations {
		processedLabelsToConfigurations[l] = ss.NewSortedSet(configurations)
	}

	matchingTargets := &MatchingTargets{
		labels:                 ss.NewSortedSetFunc(labels, CompareLabels),
		labelsToConfigurations: processedLabelsToConfigurations,
	}

	configurations, err := getConfigurationDetails(context)
	if err != nil {
		return nil, fmt.Errorf("failed to interpret configurations output: %w", err)
	}

	queryResults := &QueryResults{
		MatchingTargets:             matchingTargets,
		TransitiveConfiguredTargets: transitiveConfiguredTargets,
		TargetHashCache:             NewTargetHashCache(transitiveConfiguredTargets, bazelRelease),
		BazelRelease:                bazelRelease,
		QueryError:                  nil,
		configurations:              configurations,
	}
	return queryResults, nil
}

func runToCqueryResult(context *Context, pattern string) (*analysis.CqueryResult, error) {
	log.Printf("Running cquery on %s", pattern)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	returnVal, err := context.BazelCmd.Execute(
		BazelCmdConfig{Dir: context.WorkspacePath, Stdout: &stdout, Stderr: &stderr},
		[]string{"--output_base", context.BazelOutputBase}, "cquery", "--output=proto", pattern)

	if returnVal != 0 || err != nil {
		return nil, fmt.Errorf("failed to run cquery on %s: %w. Stderr:\n%v", pattern, err, stderr.String())
	}

	content := stdout.Bytes()

	var result analysis.CqueryResult
	if err := proto.Unmarshal(content, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cquery stdout: %w", err)
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
		l, err := labelOf(target.GetTarget())
		if err != nil {
			return nil, err
		}

		_, ok := configuredTargets[l]
		if !ok {
			configuredTargets[l] = make(map[Configuration]*analysis.ConfiguredTarget)
		}

		configuredTargets[l][Configuration(target.GetConfiguration().GetChecksum())] = target
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
