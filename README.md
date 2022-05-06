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
  -target-pattern string
    	Target pattern to diff. (default "//...")
  -verbose
    	Whether to explain (messily) why each target is getting run
  -working-directory string
    	Working directory to query (default ".")
```

This binary lists targets to stdout, one-per-line, which were affected between <before-revision> and the currently checked-out revision.

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
  -target-pattern string
    	Target pattern to consider. (default "//...")
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

## Contributing

Contributions are very welcome!

We have an extensive integration testing suite in the `tests/integration` directory which is as its own README. Please make sure any contributions are covered by a test.

When adding new dependencies to the Go code, please run `scripts/update-dependencies`.

In general, BUILD files in this repo are maintained by `gazelle`; to regenerate tem, please run `bazel run //:gazelle`.

Alongside each `go_proto_library`, there is a runnable `copy_proto_output` rule which can be used to generate the Go source for a protobuf, in case it's useful to inspect.
