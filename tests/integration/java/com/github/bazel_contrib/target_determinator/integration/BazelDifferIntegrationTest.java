package com.github.bazel_contrib.target_determinator.integration;

import static junit.framework.TestCase.fail;

import com.github.bazel_contrib.target_determinator.label.Label;
import com.google.common.base.Joiner;
import com.google.common.collect.ImmutableSet;
import java.io.File;
import java.io.IOException;
import java.lang.ProcessBuilder.Redirect;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Set;
import org.junit.Ignore;

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
    processBuilder.environment().put("PATH", System.getenv("PATH"));
    try {
      if (processBuilder.start().waitFor() != 0) {
        throw new TargetComputationErrorException(
            String.format("Expected exit code 0 when running %s", Joiner.on(" ").join(argv)),
            ImmutableSet.of());
      }
    } catch (IOException | InterruptedException e) {
      throw new RuntimeException(e);
    }
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

  // Returning things in //external

  @Override
  public void unconsumedIndirectWorkspaceChangeIsNoOp() {
    allowOverBuilds("bazel-differ returns targets in //external as changed");
    super.unconsumedIndirectWorkspaceChangeIsNoOp();
  }

  @Override
  public void movingStarlarkRuleToExternalRepoIsNoOp() {
    allowOverBuilds("bazel-differ returns targets in //external as changed");
    super.movingStarlarkRuleToExternalRepoIsNoOp();
  }

  @Override
  public void modifyingRuleViaWorkspaceFile() {
    allowOverBuilds("bazel-differ returns targets in //external as changed");
    super.modifyingRuleViaWorkspaceFile();
  }

  @Override
  public void changingFileLoadedByWorkspaceTriggersTargets() {
    allowOverBuilds("bazel-differ returns targets in //external as changed");
    super.changingFileLoadedByWorkspaceTriggersTargets();
  }

  // Bazel version changes

  @Override
  @Ignore("bazel-differ doesn't seem to track bazel versions.")
  public void changedBazelVersion_native() {}

  @Override
  @Ignore("bazel-differ doesn't seem to track bazel versions.")
  public void changedBazelVersion_starlark() {}

  // Different behaviour with respect to errors

  @Override
  public void reducingVisibilityOnDependencyAffectsTarget() {
    expectFailure();
    doTest(
        Commits.ADD_INDIRECTION_FOR_SIMPLE_JAVA_LIBRARY,
        Commits.REDUCE_DEPENDENCY_VISIBILITY,
        // bazel-differ doesn't return any targets on failure.
        Set.of());
  }

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
  public void explicitlySpecifyingDefaultValueDoesNotTrigger_native() {
    allowOverBuilds("bazel-differ isn't aware of attribute defaults.");
    super.explicitlySpecifyingDefaultValueDoesNotTrigger_native();
  }

  @Override
  public void refactoringStarlarkRuleIsNoOp() {
    allowOverBuilds(
        "Rule implementation attr factors in hashes of entire transitively loaded bzl files, rather"
            + " than anything more granular or processed");
    super.refactoringStarlarkRuleIsNoOp();
  }
}
