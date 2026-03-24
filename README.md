# Target Determinator

Target determinator is a binary (and Go API) used to determine which Bazel targets changed between two git commits.

## target-determinator binary

For simple listing, the `target-determinator` binary is supplied:

```
Usage of bazel-bin/target-determinator/target-determinator_/target-determinator:
  -analysis-cache-clear-strategy string
        Strategy for clearing the analysis cache. Accepted values: skip,shutdown,discard. (default "skip")
  -bazel string
        Bazel binary (basename on $PATH, or absolute or relative path) to run. (default "bazel")
  -bazel-opts value
        Options to pass to Bazel. Assumed to apply to build and cquery. Options should use relative paths for repository
        files (see --bazel-startup-opts).
  -bazel-startup-opts value
        Startup options to pass to Bazel. Options such as '--bazelrc' should use relative paths for files under the
        repository to avoid issues (TD may check out the repository in a temporary directory).
  -before-query-error-behavior string
        How to behave if the 'before' revision query fails. Accepted values: fatal,ignore-and-build-all (default
        "ignore-and-build-all")
  -cache-dir string
        Cache directory to avoid existing re-computations. Note: home- and system- bazelrc files, environment variables,
        and host hardware/OS are not included in the results cache key. Use --nocache_results if necessary. (default
        "/Users/rchossart/.cache/target-determinator")
  -compare-queries-around-analysis-cache-clear
        Whether to check for query result differences before and after analysis cache clears. This is a temporary flag
        for performing real-world analysis.
  -delete-cached-worktree
        Delete created worktrees after use when created. Keeping them can make subsequent invocations faster.
  -enforce-clean value
        Pass --enforce-clean=enforce-clean to fail if the repository is unclean, or --enforce-clean=allow-ignored to
        allow ignored untracked files (the default). (default allow-ignored)
  -filter-incompatible-targets
        Whether to filter out incompatible targets from the candidate set of affected targets. (default true)
  -ignore-file value
        Files to ignore for git operations, relative to the working-directory. These files shan't affect the Bazel
        graph.
  -nocache_results
        Disable loading and saving of results to the cache.
  -targets bazel query
        Targets to consider. Accepts any valid bazel query expression (see https://bazel.build/reference/query).
        (default "//...")
  -verbose
        Whether to explain (messily) why each target is getting run
  -version
        Print the version of the tool and exit.
  -working-directory string
        Working directory to query. (default ".")
```

This binary lists targets to stdout, one-per-line, which were affected between <before-revision> and the currently checked-out revision.

## hash-persister and hash-differ binaries

Target Determinator now includes hash persistence capabilities for optimized CI workflows:

- **hash-persister**: Computes and saves target hashes for a specific git commit to a JSON file, enabling efficient comparison without recomputing hashes later.
- **hash-differ**: Compares two persisted hash files to identify changed, added, or removed targets between commits, supporting multiple output formats (JSON, target list, summary).

These tools enable faster target determination in CI by pre-computing hashes once per commit and reusing them across multiple comparisons.

## driver binary

`driver` is a binary which implements a simple CI pipeline; it runs the same logic as `target-determinator`, then tests all identified targets.

```
Usage of driver:
  driver <before-revision>
Where <before-revision> may be any commit-like strings - full commit hashes, short commit hashes, tags, branches, etc.
Optional flags:
  -bazel string
    	Bazel binary (basename on $PATH, or absolute or relative path) to run (default "bazel")
  -ignore-file value
    	Files to ignore for git operations, relative to the working-directory. These files shan't affect the Bazel graph.
  -manual-test-mode string
    	How to handle affected tests tagged manual. Possible values: run|skip (default "skip")
  -targets bazel query
      Targets to consider. Accepts any valid bazel query expression (see https://bazel.build/reference/query). (default "//...")
  -working-directory string
    	Working directory to query (default ".")
```

## WalkAffectedTargets API

Both of the above binaries are thin wrappers around a Go function called `WalkAffectedTargets` which calls a user-supplied callback for each affected target between two commits:

```go
// WalkAffectedTargets computes which targets have changed between two commits, and calls
// callback once for each target which has changed.
// Explanation of the differences may be expensive in both time and memory to compute, so if
// includeDifferences is set to false, the []Difference parameter to the callback will always be nil.
func WalkAffectedTargets(context *Context, commitishBefore, commitishAfter LabelledGitRev, pattern label.Pattern, includeDifferences bool, callback WalkCallback) error { ... }

type WalkCallback func(label.Label, []Difference, *analysis.ConfiguredTarget)
```

This can be used to flexibly build your own logic handling the affected targets to drive whatever analysis you want.

## Caching

Target Determinator caches the results of Bazel cquery invocations across runs. On a cache hit, the expensive cquery and hashing work for a given commit is skipped entirely.

The cache key is derived from:

- The target-determinator binary itself (SHA-256 hash)
- The Bazel version (`bazel info release`)
- The git tree SHA of the queried commit
- The target pattern (e.g. `//...`)
- CLI options that may affect cquery results, such as `--filter-incompatible-targets` and the Bazel startup/build options passed via `--bazel-startup-opts` / `--bazel-opts`

*Not* included in the cache key:

- User and system bazelrc files (`~/.bazelrc`, `/etc/bazel.bazelrc`, and files they import)
- The host machine (hardware, OS). Cache entries produced on one machine are not guaranteed to be valid on another (e.g. a different CPU architecture can change which platform-constrained targets are selected). Do not share the cache directory across machines.
- Environment variables, whether they are used by Bazel or not.

### Environment variables and caching

Without caching, the "before" and "after" cquery calls are both made with the same environment variables. Taken in the context of a CI pipeline run, for example, this means that even the "before" computation uses the *current* (or "after") environment variables, not the environment variables that existed when the "before" commit was built. That answers the question "what targets differ between these two commits, assuming the environment was the same?".

With caching, however, the "before" result may have been computed in an earlier pipeline run, under the environment variables that were in effect *at that time*. If an environment variable affected Bazel's query output (e.g. because it is referenced by `--workspace_status`, `--action_env`, `--test_env`, or a repo rule), the cached result reflects the old environment, while the "after" result reflects the new one. The two results are then compared under different conditions, which may produce spurious differences.

In practice this matters most in release pipelines where stamping or versioning variables (e.g. `MY_PKG_VERSION`) change between runs. If you want to answer "which targets would have changed, assuming the environment is the same before and after?", run `target-determinator` with `--nocache_results` to force both computations to happen in the same environment.

## How to get Target Determinator

Pre-built binary releases are published as [GitHub Releases](https://github.com/bazel-contrib/target-determinator/releases) for most changes.

We recommend you download the latest release for your platform, and run it where needed (e.g. in your CI pipeline).

We avoid breaking changes where possible, but offer no formal compatibility guarantees release-to-release.

We do not recommend integrating Target Determinator into your Bazel build graph unless you have a compelling reason to do so.

## Contributing

Contributions are very welcome!

We have an extensive integration testing suite in the `tests/integration` directory which has its own README. The test suite also runs against several other target determinator implementations which are pulled in as `http_archive`s, to test compatibility. Please make sure any contributions are covered by a test.

When adding new dependencies to the Go code, please run `scripts/update-dependencies`.

In general, BUILD files in this repo are maintained by `gazelle`; to regenerate tem, please run `bazel run //:gazelle`.

Alongside each `go_proto_library`, there is a runnable `copy_proto_output` rule which can be used to generate the Go source for a protobuf, in case it's useful to inspect.

## Supported Bazel versions

Target Determinator currently supports Bazel 4.0.0 up to and including the latest LTS release.

We are happy to support newer Bazel versions (as long as this doesn't break support for the current LTS), but only exhaustively test against the latest LTS release.

We have a small number of smoke tests which verify basic functionality on the oldest supported release, but do not regularly test against it.
