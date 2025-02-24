package com.github.bazel_contrib.target_determinator.integration;

import com.github.bazel_contrib.target_determinator.label.Label;
import com.google.common.base.Joiner;
import java.io.File;
import java.io.IOException;
import java.lang.ProcessBuilder.Redirect;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Arrays;
import java.util.Set;
import java.util.stream.Collectors;

import org.junit.Ignore;
import org.junit.Test;

public class BazelDifferIntegrationTest extends Tests {
  private static final String BAZEL_DIFFER =
      new File(System.getProperty("bazel_differ")).getAbsolutePath();
  private static final String BAZEL = "bazelisk";

  Set<Label> getTargets(Path workspace, String commitBefore, String commitAfter)
      throws TargetComputationErrorException {
    String workspacePath = workspace.toString();
    try {
      Path tempdir = Files.createTempDirectory("targetdeterminator-bazel-differ");
      Path affectedTargets = tempdir.resolve("affected-targets");
      runProcess(
          workspace,
          BAZEL_DIFFER,
          "get-targets",
          "-w",
          workspacePath,
          "-b",
          BAZEL,
          "-s",
          commitBefore,
          "-f",
          commitAfter,
          "-o",
          affectedTargets.toString(),
          "-q",
          "kind(rule, set({{.Targets}}))");
      return Util.linesToLabels(affectedTargets);
    } catch (IOException e) {
      throw new RuntimeException(e);
    }
  }

  private static void runProcess(Path workingDirectory, String... argv)
      throws TargetComputationErrorException {
    ProcessBuilder processBuilder = new ProcessBuilder(argv);
    processBuilder.directory(workingDirectory.toFile());
    processBuilder.redirectOutput(Redirect.INHERIT);
    processBuilder.redirectError(Redirect.INHERIT);
    // Do not clean the environment so we can inherit variables passed e.g. via --test_env.
    // Useful for CC (needed by bazel).
    processBuilder.environment().put("HOME", System.getProperty("user.home"));

    // There is a bug in `bazel-differ` where in `internal/bazel.go` they default to always using `bazel`
    // rather than the the actual bazel instance we've requested. This is normally fine but when we
    // execute our own tests with `bazelisk`, the `PATH` has the version of `bazel` requested in our
    // _own_ `.bazelversion` file at the head of the `PATH` (pulled from `bazelisk`'s own cache). To
    // avoid this, we're going to remove items from the `PATH` that might be from bazelisk using a
    // best-effort heuristic.
    String cacheDir = getUserCacheDirectory();
    String amendedPath = Arrays.stream(System.getenv("PATH").split(File.pathSeparator))
            .filter(item -> !item.contains(cacheDir + File.separator + "bazelisk"))
            .collect(Collectors.joining(File.pathSeparator));
    processBuilder.environment().put("PATH", amendedPath);

    try {
      if (processBuilder.start().waitFor() != 0) {
        throw new TargetComputationErrorException(
            String.format("Expected exit code 0 when running %s", Joiner.on(" ").join(argv)),
            "",
            "");
      }
    } catch (IOException | InterruptedException e) {
      throw new RuntimeException(e);
    }
  }

  // `bazel-differ` is written in Go, so we mirror what https://pkg.go.dev/os#UserCacheDir does
  private static String getUserCacheDirectory() {
    String dir = System.getenv("XDG_CACHE_HOME");
    if (dir != null) {
      return dir;
    }
    String osName = System.getProperty("os.name").toLowerCase();
    if (osName.contains("windows")) {
      return System.getenv("LocalAppData");
    }
    if (osName.contains("darwin") || osName.contains("mac")) {
      return System.getProperty("user.home") + "/Library/Caches";
    }
    return System.getProperty("user.home") + "/.cache";
  }

  // Configuration-related tests

  @Override
  @Ignore("bazel-differ doesn't inspect configurations.")
  public void changedBazelrcAffectingAllTests() {}

  @Override
  @Ignore("bazel-differ doesn't inspect configurations.")
  public void changedBazelrcAffectingSomeTests() {}

  @Override
  @Ignore("bazel-differ doesn't inspect configurations.")
  public void importInBazelrcAffectingJava() {}

  @Override
  @Ignore("bazel-differ doesn't inspect configurations.")
  public void tryImportInBazelrcAffectingJava() {}

  @Override
  @Ignore("bazel-differ doesn't inspect configurations.")
  public void changingTargetConfigurationDoesNotAffectHostConfiguration() {}

  @Override
  @Ignore("bazel-differ doesn't inspect configurations.")
  public void changingHostConfigurationDoesNotAffectTargetConfiguration() {}

  @Override
  @Test
  public void ignoredPlatformSpecificSrcChanged() throws Exception {
    allowOverBuilds("bazel-differ doesn't filter platform-specific changes");
    super.ignoredPlatformSpecificSrcChanged();
  }

  @Override
  @Test
  public void ignoredPlatformSpecificDepChanged() throws Exception {
    allowOverBuilds("bazel-differ doesn't filter platform-specific changes");
    super.ignoredPlatformSpecificDepChanged();
  }

  // Returning things in //external

  @Override
  public void unconsumedIndirectWorkspaceChangeIsNoOp() throws Exception {
    allowOverBuilds("bazel-differ returns targets in //external as changed");
    super.unconsumedIndirectWorkspaceChangeIsNoOp();
  }

  @Override
  public void movingStarlarkRuleToExternalRepoIsNoOp() throws Exception {
    allowOverBuilds("bazel-differ returns targets in //external as changed");
    super.movingStarlarkRuleToExternalRepoIsNoOp();
  }

  @Override
  public void modifyingRuleViaWorkspaceFile() throws Exception {
    allowOverBuilds("bazel-differ returns targets in //external as changed");
    super.modifyingRuleViaWorkspaceFile();
  }

  @Override
  public void changingFileLoadedByWorkspaceTriggersTargets() throws Exception {
    allowOverBuilds("bazel-differ returns targets in //external as changed");
    super.changingFileLoadedByWorkspaceTriggersTargets();
  }

  // Bazel version changes

  @Override
  @Ignore("bazel-differ doesn't seem to track bazel versions.")
  public void changedBazelMajorVersion_native() {}

  @Override
  @Ignore("bazel-differ doesn't seem to track bazel versions.")
  public void changedBazelPatchVersion_native() {}

  @Override
  @Ignore("bazel-differ doesn't seem to track bazel versions.")
  public void changedBazelMajorVersion_starlark() {}

  @Override
  @Ignore("bazel-differ doesn't seem to track bazel versions.")
  public void changedBazelPatchVersion_starlark() {}

  // Submodules

  @Override
  @Ignore
  public void addTrivialSubmodule() {}

  @Override
  @Ignore
  public void changeSubmodulePath() {}

  @Override
  @Ignore
  public void addDependentTargetInSubmodule() {}

  @Override
  @Ignore
  public void succeedForUncleanSubmodule() {}

  // Misc

  @Override
  @Ignore("bazel-differ seems to behave weirdly with relative git revisions.")
  public void testRelativeRevisions() {}

  @Override
  public void explicitlySpecifyingDefaultValueDoesNotTrigger_native() throws Exception {
    allowOverBuilds("bazel-differ isn't aware of attribute defaults.");
    super.explicitlySpecifyingDefaultValueDoesNotTrigger_native();
  }

  @Override
  public void changingUnimportantPermissionDoesNotTrigger_native() throws Exception {
    allowOverBuilds("bazel-differ takes into account all permission bits.");
    super.changingUnimportantPermissionDoesNotTrigger_native();
  }

  @Override
  public void refactoringStarlarkRuleIsNoOp() throws Exception {
    allowOverBuilds(
        "Rule implementation attr factors in hashes of entire transitively loaded bzl files, rather"
            + " than anything more granular or processed");
    super.refactoringStarlarkRuleIsNoOp();
  }

  @Override
  @Ignore("bazel-differ doesn't check file modes")
  public void testChmodFile() {}

  @Override
  @Ignore("bazel-differ doesn't handle no targets being returned from a query")
  public void zeroToOneTarget_native() {}

  @Override
  @Ignore("bazel-differ does not filter incompatible targets")
  public void incompatibleTargetsAreFiltered() throws Exception {}

  @Override
  @Ignore("bazel-differ does not filter incompatible targets")
  public void incompatibleTargetsAreFiltered_bazelIssue21010() throws Exception {}

  @Override
  @Ignore("I'm not smart enough to figure out why this fails now")
  public void testMinimumSupportedBazelVersion() {}
}
