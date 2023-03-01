# Target Determinator Tests

Tests edge-cases of detecting which targets to run when changing bazel graphs.

There is a companion git repo hosted at https://github.com/bazel-contrib/target-determinator-testdata with a series of changes on assorted tags.

The general structure of each test is to hand the determinator two commits, and ask it which targets should be tested. Each test will assert that:
1. At least the required tests were detected.
2. None of the forbidden tests were detected.
3. No extra tests were detected.

It is reasonable, but not preferable, for the "extra tests detected" check to fail - this will lead to over-building. The other two assertions are very important.

## Gaps

There is currently no coverage for:
* What should happen in the event of invalid build graphs. There are multiple possibly desirable behaviours ("Just error", "Build everything", "Give the best list we can", "Best effort + error signal"). If we agree on one, we can codify it.
* Platform-specific changes. The test suite assumes that wherever the test suite is being run is an equivalent platform to where the tests will actually be run.

## Adding more target determinators

We have two other tools which solve the same problem wired up to our shared test suite:

* [bazel-diff](https://github.com/Tinder/bazel-diff) 
* [bazel-differ](https://github.com/ewhauser/bazel-differ)

Extra tools can be supported by adding a new class to `com.github.bazel_contrib.target_determinator.integration` which inherits from `Tests` and implementing the abstract `getTargets` method as per its javadoc.

## How to handle differences in expectations/behaviors

Each supported target determinator has its own subclass of `Tests`. `Tests` contains tests which should apply to all target determinators. Implementation-specific tests can be added to subclasses, or to separate unrelated classes.

For places where more targets are returned than expected, the subclass should override the individual test method, and call the method `allowOverBuilds` with an explanation of why this is expected, before delegating to the superclass.

For places where a target determinator has a different, equally valid, interpretation of what should be returned, the test method can simply be overridden.

For places where behavior is not supported, or simply incorrect, the overridden test method should be annotated with an `@Ignore` annotation, with an explanation of why.

## Adding new tests

Adding tests is slightly fiddly, as it involves making coordinated changes across the testdata repo and this one.

This is roughly the flow to add tests:
1. Decide whether this is specific to this TD implementation, or applies generally to all target determinators. If it's general, the new test should go in [java/com/github/bazel_contrib/target_determinator/integration/Tests.java], otherwise in [java/com/github/bazel_contrib/target_determinator/integration/TargetDeterminatorSpecificFlagsTest.java] - in both cases, the existing tests should be easy to crib from.
1. If new commits are needed in the testdata repo (which is most often the case), clone that, and add commits to it. Each test commit is typically in a unique branch based on https://github.com/bazel-contrib/target-determinator-testdata/commit/6682820b4acb455f13bc3cf8f7d254056092e306 - we try to have each branch have the minimal changes needed for each test, rather than amassing lots of unrelated changes on fewer branches.
   When merging new commits to the testdata repo, we create two refs per added commit - a branch (which may be rewritten in the future), and an immutable tag which will stay around forever. Say we're adding a commit testing upper-case target names, we may call the branch `upper-case-targets`, and we'll create the tag `v0/upper-case-targets` matching it. If we change the branch in the future (e.g. to change the bazel version), we'll rewrite history on the branch, and create a new tag `v1/upper-case-targets`.
1. For actually testing out your new test locally, you can edit [java/com/github/bazel_contrib/target_determinator/integration/TestdataRepo.java#L18](the TestdataRepo helper class in this repo) to clone from a `file://` URI pointing at your local clone. You probably also want to call `.setCloneAllBranches(true)` on the `Git.cloneRepository` call, otherwise your work-in-progress branches won't be cloned when you run the tests

When sending out new tests for review, feel free to set the clone URI to your fork on GitHub (so the tests actually pass), and include in your PR which commits/branches need to be upstreamed into the testdata repo. The reviewer will push these commits when the code otherwise looks good, and ask you to revert back to the upstream URI.
