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
