# Target Determinator

Target determinator is a binary (and Go API) used to determine which Bazel targets changed between two git commits.

## target-determinator binary

For simple listing, the `target-determinator` binary is supplied:

```
Usage of target-determinator:
target-determinator <before-revision>
Where <before-revision> may be any commit revision - full commit hashes, short commit hashes, tags, branches, etc.
  -bazel string
    	Bazel binary (basename on $PATH, or absolute or relative path) to run (default "bazel")
  -ignore-file value
    	Files to ignore for git operations, relative to the working-directory. These files shan't affect the Bazel graph.
  -targets bazel query
      Targets to consider. Accepts any valid bazel query expression (see https://bazel.build/reference/query). (default "//...")
  -verbose
    	Whether to explain (messily) why each target is getting run
  -working-directory string
    	Working directory to query (default ".")
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
