package com.github.bazel_contrib.target_determinator.integration;

import com.github.bazel_contrib.target_determinator.label.Label;
import com.google.common.base.Joiner;
import java.io.File;
import java.io.IOException;
import java.lang.ProcessBuilder.Redirect;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Set;
import org.junit.Ignore;
import org.junit.Test;

public class BazelDiffIntegrationTest extends Tests {
  private static final String BAZEL_DIFF =
      new File(System.getProperty("bazel_diff")).getAbsolutePath();
  private static final String BAZEL = "bazelisk";

  Set<Label> getTargets(Path workspace, String commitBefore, String commitAfter)
      throws TargetComputationErrorException {
    String workspacePath = workspace.toString();
    try {
      Path tempdir = Files.createTempDirectory("targetdeterminator-bazel-diff");

      String hashesBefore = tempdir.resolve("hashes-before").toString();
      String hashesAfter = tempdir.resolve("hashes-after").toString();
      Path affectedTargets = tempdir.resolve("affected-targets");

      runProcess(workspace, "git", "checkout", "--quiet", commitBefore);
      runProcess(
          workspace, BAZEL_DIFF, "generate-hashes", "-w", workspacePath, "-b", BAZEL, hashesBefore);
      runProcess(workspace, "git", "checkout", "--quiet", commitAfter);
      runProcess(
          workspace, BAZEL_DIFF, "generate-hashes", "-w", workspacePath, "-b", BAZEL, hashesAfter);
      runProcess(
          workspace,
          BAZEL_DIFF,
          "-sh",
          hashesBefore,
          "-fh",
          hashesAfter,
          "-w",
          workspacePath,
          "-b",
          BAZEL,
          "-o",
          affectedTargets.toString());
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
            "",
            "");
      }
    } catch (IOException | InterruptedException e) {
      throw new RuntimeException(e);
    }
  }

  // Configuration-related tests

  @Override
  @Ignore("bazel-diff doesn't inspect configurations.")
  public void changedBazelrcAffectingAllTests() {}

  @Override
  @Ignore("bazel-diff doesn't inspect configurations.")
  public void changedBazelrcAffectingSomeTests() {}

  @Override
  @Ignore("bazel-diff doesn't inspect configurations.")
  public void importInBazelrcAffectingJava() {}

  @Override
  @Ignore("bazel-diff doesn't inspect configurations.")
  public void tryImportInBazelrcAffectingJava() {}

  @Override
  @Ignore("bazel-diff doesn't seem to track bazel versions.")
  public void changedBazelPatchVersion_native() {}

  @Override
  @Ignore("bazel-diff doesn't seem to track bazel versions.")
  public void changedBazelPatchVersion_starlark() {}

  @Override
  @Ignore("bazel-diff doesn't inspect configurations.")
  public void changingTargetConfigurationDoesNotAffectHostConfiguration() {}

  @Override
  @Ignore("bazel-diff doesn't inspect configurations.")
  public void changingHostConfigurationDoesNotAffectTargetConfiguration() {}

  @Override
  @Test
  public void ignoredPlatformSpecificSrcChanged() throws Exception {
    allowOverBuilds("bazel-diff doesn't filter platform-specific changes");
    super.ignoredPlatformSpecificSrcChanged();
  }

  @Override
  @Test
  public void ignoredPlatformSpecificDepChanged() throws Exception {
    allowOverBuilds("bazel-diff doesn't filter platform-specific changes");
    super.ignoredPlatformSpecificDepChanged();
  }

  // Submodules

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
  @Ignore("bazel-diff seems to behave weirdly with relative git revisions.")
  public void testRelativeRevisions() {}

  @Override
  public void explicitlySpecifyingDefaultValueDoesNotTrigger_native() throws Exception {
    allowOverBuilds("bazel-diff isn't aware of attribute defaults.");
    super.explicitlySpecifyingDefaultValueDoesNotTrigger_native();
  }

  @Override
  public void refactoringStarlarkRuleIsNoOp() throws Exception {
    allowOverBuilds(
        "Rule implementation attr factors in hashes of entire transitively loaded bzl files, rather"
            + " than anything more granular or processed");
    super.refactoringStarlarkRuleIsNoOp();
  }

  @Override
  @Ignore("bazel-diff doesn't check file modes")
  public void testChmodFile() {}

  @Override
  @Ignore("bazel-diff does not filter incompatible targets")
  public void incompatibleTargetsAreFiltered() throws Exception {}

  @Override
  @Ignore("bazel-diff does not filter incompatible targets")
  public void incompatibleTargetsAreFiltered_bazelIssue21010() throws Exception {}
}
