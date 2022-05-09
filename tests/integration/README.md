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
