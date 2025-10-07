package pkg

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/user"
	path2 "path"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/aristanetworks/goarista/path"
	"github.com/bazel-contrib/target-determinator/common"
	ss "github.com/bazel-contrib/target-determinator/common/sorted_set"
	"github.com/bazel-contrib/target-determinator/common/versions"
	"github.com/bazel-contrib/target-determinator/third_party/protobuf/bazel/analysis"
	"github.com/bazel-contrib/target-determinator/third_party/protobuf/bazel/build"
	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/hashicorp/go-version"
	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/proto"
)

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
	// BeforeQueryErrorBehavior describes how to handle errors when querying the "before" revision.
	// Accepted values are:
	// - "fatal" - treat an error querying as fatal.
	// - "ignore-and-build-all" - ignore the error, and build all targets at the "after" revision.
	BeforeQueryErrorBehavior string
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
	// FilterIncompatibleTargets controls whether we filter out incompatible targets from the candidate set of affected targets.
	FilterIncompatibleTargets bool
	// EnforceCleanRepo controls whether we should fail if the repository is unclean.
	EnforceCleanRepo bool
}

// FullyProcess returns the before and after metadata maps, with fully filled caches.
func FullyProcess(context *Context, revBefore LabelledGitRev, revAfter LabelledGitRev, targets TargetsList) (*QueryResults, *QueryResults, error) {
	log.Printf("Processing %s", revBefore)
	queryInfoBefore, err := fullyProcessRevision(context, revBefore, targets)
	if err != nil {
		if queryInfoBefore == nil {
			return nil, nil, err
		} else {
			if context.BeforeQueryErrorBehavior == "ignore-and-build-all" {
				log.Printf("A query error occurred querying %s - ignoring the error and treating all matching targets from the '%s' revision as affected. Error querying: %v", revBefore, revAfter.Label, err)
			} else {
				return nil, nil, fmt.Errorf("error occurred querying %s: %w", revBefore, err)
			}
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
			err = fmt.Errorf("failed to check out original commit during cleanup: %v", innerErr)
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
		BeforeQueryErrorBehavior:               context.BeforeQueryErrorBehavior,
		AnalysisCacheClearStrategy:             context.AnalysisCacheClearStrategy,
		CompareQueriesAroundAnalysisCacheClear: context.CompareQueriesAroundAnalysisCacheClear,
		FilterIncompatibleTargets:              context.FilterIncompatibleTargets,
		EnforceCleanRepo:                       context.EnforceCleanRepo,
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
//   - the original worktree is unclean (non-ignored untracked files or tracked local changes).
//   - upon checking out the new revision, the worktree is unclean. This can happen when a submodule
//     was moved or removed between the current and target commit, or when the contents of the
//     .gitignore file changes.
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
		if context.EnforceCleanRepo {
			return "", fmt.Errorf("repository was not clean before checking out %v", rev)
		}

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
			if context.EnforceCleanRepo {
				return "", fmt.Errorf("repository was not clean after checking out %v", rev)
			}

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

func retrieveRepoMapping(workspacePath string, bazelCmd BazelCmd) (map[string]string, error) {
	var stdoutBuf, stderrBuf bytes.Buffer

	result, err := bazelCmd.Execute(
		BazelCmdConfig{Dir: workspacePath, Stdout: &stdoutBuf, Stderr: &stderrBuf},
		nil, "mod", "dump_repo_mapping", "")

	if result != 0 || err != nil {
		log.Printf("failed to get the Bazel repository mapping: %v. Stderr:\n%v", err, stderrBuf.String())
		return nil, err
	}

	var repoMapping map[string]string
	unmarshalErr := json.Unmarshal(stdoutBuf.Bytes(), &repoMapping)
	if unmarshalErr != nil {
		log.Printf("failed to unmarshal the Bazel repository mapping: %v", err)
		return nil, unmarshalErr
	}

	return repoMapping, nil

}

func NormalizeConfiguredTarget(target *analysis.ConfiguredTarget, n *Normalizer) {
	if target.GetTarget().GetRule() != nil {
		rule := target.GetTarget().GetRule()
		for _, attr := range rule.GetAttribute() {
			n.NormalizeAttribute(attr)
		}

		for idx, input := range rule.GetConfiguredRuleInput() {
			lbl, err := n.ParseCanonicalLabel(*input.Label)
			if err != nil {
				value := lbl.String()
				rule.GetConfiguredRuleInput()[idx].Label = &value
			}
		}
	}
}

// Note that a non-nil QueryResults may be returned even in the error case, which will have an
// empty target-set, but may contain other useful information (e.g. the bazel release version).
// Checking for nil-ness of the error is the true arbiter for whether the entire query was successful.
func doQueryDeps(context *Context, targets TargetsList) (*QueryResults, error) {
	bazelRelease, err := BazelRelease(context.WorkspacePath, context.BazelCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve the bazel release: %w", err)
	}

	// The `bazel mod dump_repo_mapping` subcommand was added in Bazel 7.1.2.
	canRetrieveMapping, _ := versions.ReleaseIsInRange(bazelRelease, version.Must(version.NewVersion("7.1.2")), nil)

	hasBzlmod, err := IsBzlmodEnabled(context.WorkspacePath, context.BazelCmd, bazelRelease)
	if err != nil {
		return nil, fmt.Errorf("failed to determine if bzlmod is enabled: %w", err)
	}

	var repoMapping map[string]string
	if hasBzlmod && (canRetrieveMapping != nil && *canRetrieveMapping) {
		var retrieveErr error
		repoMapping, retrieveErr = retrieveRepoMapping(context.WorkspacePath, context.BazelCmd)
		if retrieveErr != nil {
			return nil, fmt.Errorf("failed to retrieve bazel dump repo mapping: %w", retrieveErr)
		}
	} else {
		repoMapping = map[string]string{}
	}

	normalizer := Normalizer{repoMapping}

	// Work around https://github.com/bazelbuild/bazel/issues/21010
	var incompatibleTargetsToFilter map[label.Label]bool
	hasIncompatibleTargetsBug, explanation := versions.ReleaseIsInRange(bazelRelease, version.Must(version.NewVersion("7.0.0-pre.20230628.2")), version.Must(version.NewVersion("7.4.0")))
	if hasIncompatibleTargetsBug != nil && *hasIncompatibleTargetsBug {
		if !context.FilterIncompatibleTargets {
			return nil, fmt.Errorf("requested not to filter incompatible targets, but bazel version %s has a bug requiring filtering incompatible targets - see https://github.com/bazelbuild/bazel/issues/21010", bazelRelease)
		}
		incompatibleTargetsToFilter, err = findCompatibleTargets(context, targets.String(), false, &normalizer, bazelRelease)
		if err != nil {
			return nil, fmt.Errorf("failed to find incompatible targets: %w", err)
		}
	} else if hasIncompatibleTargetsBug == nil {
		log.Printf("Couldn't detect whether current bazel version (%s) suffers from https://github.com/bazelbuild/bazel/issues/21010: %s - assuming it does not", bazelRelease, explanation)
	}

	depsPattern := fmt.Sprintf("deps(%s)", targets.String())
	if len(incompatibleTargetsToFilter) > 0 {
		depsPattern += " - " + strings.Join(sortedStringKeys(incompatibleTargetsToFilter), " - ")
	}
	transitiveResult, err := runToCqueryResult(context, depsPattern, true, bazelRelease)
	if err != nil {
		retErr := fmt.Errorf("failed to cquery %v: %w", depsPattern, err)
		return &QueryResults{
			MatchingTargets: &MatchingTargets{
				labels:                 nil,
				labelsToConfigurations: nil,
			},
			TransitiveConfiguredTargets: nil,
			TargetHashCache:             NewTargetHashCache(nil, &normalizer, bazelRelease),
			BazelRelease:                bazelRelease,
			QueryError:                  retErr,
		}, retErr
	}

	transitiveConfiguredTargets, err := ParseCqueryResult(transitiveResult, &normalizer)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cquery result: %w", err)
	}

	matchingTargetResults, err := runToCqueryResult(context, targets.String(), false, bazelRelease)
	if err != nil {
		return nil, fmt.Errorf("failed to run top-level cquery: %w", err)
	}

	var compatibleTargets map[label.Label]bool
	if context.FilterIncompatibleTargets {
		if compatibleTargets, err = findCompatibleTargets(context, targets.String(), true, &normalizer, bazelRelease); err != nil {
			return nil, fmt.Errorf("failed to find compatible targets: %w", err)
		}
	}
	// Need to do this due to a change in bazel-gazelle & how equality between labels is determined.
	// Likely happened in https://github.com/bazel-contrib/bazel-gazelle/pull/1911.
	var compatibleTargetsStrKey = make(map[string]bool, len(compatibleTargets))
	for k, v := range compatibleTargets {
		compatibleTargetsStrKey[k.String()] = v
	}

	log.Println("Matching labels to configurations")
	labels := make([]label.Label, 0)
	labelsToConfigurations := make(map[label.Label][]Configuration)
	for _, mt := range matchingTargetResults {
		l, err := labelOf(mt.Target, &normalizer)
		if err != nil {
			return nil, fmt.Errorf("failed to parse label returned from query %s: %w", mt.Target, err)
		}
		if context.FilterIncompatibleTargets && !compatibleTargetsStrKey[l.String()] {
			continue // Ignore incompatible targets
		}
		labels = append(labels, l)

		configuration := NormalizeConfiguration(mt.Configuration.Checksum)
		labelsToConfigurations[l] = append(labelsToConfigurations[l], configuration)
	}

	processedLabelsToConfigurations := make(map[label.Label]*ss.SortedSet[Configuration], len(labels))
	for l, configurations := range labelsToConfigurations {
		processedLabelsToConfigurations[l] = ss.NewSortedSetFn(configurations, ConfigurationLess)
	}

	matchingTargets := &MatchingTargets{
		labels:                 ss.NewSortedSetFn(labels, CompareLabels),
		labelsToConfigurations: processedLabelsToConfigurations,
	}

	configurations, err := getConfigurationDetails(context)
	if err != nil {
		return nil, fmt.Errorf("failed to interpret configurations output: %w", err)
	}

	queryResults := &QueryResults{
		MatchingTargets:             matchingTargets,
		TransitiveConfiguredTargets: transitiveConfiguredTargets,
		TargetHashCache:             NewTargetHashCache(transitiveConfiguredTargets, &normalizer, bazelRelease),
		BazelRelease:                bazelRelease,
		QueryError:                  nil,
		configurations:              configurations,
	}
	return queryResults, nil
}

func sortedStringKeys[V any](m map[label.Label]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key.String())
	}
	sort.Strings(keys)
	return keys
}

func runToCqueryResult(context *Context, pattern string, includeTransitions bool, bazelRelease string) ([]*analysis.ConfiguredTarget, error) {
	log.Printf("Running cquery on %s", pattern)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	useStreamedProtoPtr, _ := versions.ReleaseIsInRange(bazelRelease, version.Must(version.NewVersion("8.2.0")), nil)
	useStreamedProto := useStreamedProtoPtr != nil && *useStreamedProtoPtr
	var args []string
	if useStreamedProto {
		args = append(args, "--output=streamed_proto")
	} else {
		args = append(args, "--output=proto")
	}
	if includeTransitions {
		args = append(args, "--transitions=lite")
	}
	args = append(args, pattern)

	returnVal, err := context.BazelCmd.Cquery(
		bazelRelease,
		BazelCmdConfig{Dir: context.WorkspacePath, Stdout: &stdout, Stderr: &stderr},
		[]string{"--output_base", context.BazelOutputBase},
		args...)

	if returnVal != 0 || err != nil {
		return nil, fmt.Errorf("failed to run cquery on %s: %w. Stderr:\n%v", pattern, err, stderr.String())
	}

	if useStreamedProto {
		var targets []*analysis.ConfiguredTarget
		unmarshalOpts := protodelim.UnmarshalOptions{MaxSize: -1}
		for {
			var singleTargetResult analysis.CqueryResult
			if err = unmarshalOpts.UnmarshalFrom(&stdout, &singleTargetResult); err == io.EOF {
				break
			} else if err != nil {
				return nil, fmt.Errorf("failed to unmarshal streamed cquery stdout: %w", err)
			}
			targets = append(targets, singleTargetResult.Results...)
		}
		return targets, nil
	} else {
		var result analysis.CqueryResult
		if err = proto.Unmarshal(stdout.Bytes(), &result); err != nil {
			return nil, fmt.Errorf("failed to unmarshal cquery stdout: %w", err)
		}
		return result.GetResults(), nil
	}
}

func findCompatibleTargets(context *Context, pattern string, compatibility bool, n *Normalizer, bazelRelease string) (map[label.Label]bool, error) {
	log.Printf("Finding compatible targets under %s", pattern)
	compatibleTargets := make(map[label.Label]bool)

	// Add the `or []` to work around https://github.com/bazelbuild/bazel/issues/17749 which was fixed in 6.2.0.
	negation := ""
	if compatibility {
		negation = "not "
	}
	queryFilter := fmt.Sprintf(` if "IncompatiblePlatformProvider" %sin (providers(target) or []) else ""`, negation)

	// Separate alias and non-alias targets to work around https://github.com/bazelbuild/bazel/issues/18421
	{
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		returnVal, err := context.BazelCmd.Cquery(
			bazelRelease,
			BazelCmdConfig{Dir: context.WorkspacePath, Stdout: &stdout, Stderr: &stderr},
			[]string{"--output_base", context.BazelOutputBase},
			fmt.Sprintf("%s - kind(alias, %s)", pattern, pattern),
			"--output=starlark",
			"--starlark:expr=target.label"+queryFilter,
		)
		if returnVal != 0 || err != nil {
			return nil, fmt.Errorf("failed to run compatibility-filtering cquery on %s: %w. Stderr:\n%v", pattern, err, stderr.String())
		}
		if err := addCompatibleTargetsLines(&stdout, compatibleTargets, n); err != nil {
			return nil, err
		}
	}

	{
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		returnVal, err := context.BazelCmd.Cquery(
			bazelRelease,
			BazelCmdConfig{Dir: context.WorkspacePath, Stdout: &stdout, Stderr: &stderr},
			[]string{"--output_base", context.BazelOutputBase},
			fmt.Sprintf("kind(alias, %s)", pattern),
			"--output=starlark",
			// Example output of `repr(target)` for an alias target: `<alias target //java/example:example_test of //java/example:OtherExampleTest>`
			"--starlark:expr=repr(target).split(\" \")[2]"+queryFilter,
		)
		if returnVal != 0 || err != nil {
			return nil, fmt.Errorf("failed to run alias compatibility-filtering cquery on %s: %w. Stderr:\n%v", pattern, err, stderr.String())
		}
		if err := addCompatibleTargetsLines(&stdout, compatibleTargets, n); err != nil {
			return nil, err
		}
	}
	return compatibleTargets, nil
}

func addCompatibleTargetsLines(r io.Reader, compatibleTargets map[label.Label]bool, n *Normalizer) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		labelStr := scanner.Text()
		if labelStr == "" {
			continue
		}
		label, err := n.ParseCanonicalLabel(labelStr)
		if err != nil {
			return fmt.Errorf("failed to parse label from compatibility-filtering: %q: %w", labelStr, err)
		}
		compatibleTargets[label] = true
	}

	return nil
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

func ParseCqueryResult(targets []*analysis.ConfiguredTarget, n *Normalizer) (map[label.Label]map[Configuration]*analysis.ConfiguredTarget, error) {
	configuredTargets := make(map[label.Label]map[Configuration]*analysis.ConfiguredTarget, len(targets))

	for _, target := range targets {
		l, err := labelOf(target.GetTarget(), n)
		if err != nil {
			return nil, err
		}

		_, ok := configuredTargets[l]
		if !ok {
			configuredTargets[l] = make(map[Configuration]*analysis.ConfiguredTarget)
		}

		NormalizeConfiguredTarget(target, n)

		configuredTargets[l][NormalizeConfiguration(target.GetConfiguration().GetChecksum())] = target
	}

	return configuredTargets, nil
}

func labelOf(target *build.Target, n *Normalizer) (label.Label, error) {
	switch target.GetType() {
	case build.Target_RULE:
		return n.ParseCanonicalLabel(target.GetRule().GetName())
	case build.Target_SOURCE_FILE:
		return n.ParseCanonicalLabel(target.GetSourceFile().GetName())
	case build.Target_GENERATED_FILE:
		return n.ParseCanonicalLabel(target.GetGeneratedFile().GetName())
	case build.Target_PACKAGE_GROUP:
		return n.ParseCanonicalLabel(target.GetPackageGroup().GetName())
	case build.Target_ENVIRONMENT_GROUP:
		return n.ParseCanonicalLabel(target.GetEnvironmentGroup().GetName())
	default:
		return label.NoLabel, fmt.Errorf("labelOf called on unknown target type: %v", target.GetType().String())
	}
}

func CompareLabels(a, b label.Label) bool {
	return a.String() < b.String()
}
