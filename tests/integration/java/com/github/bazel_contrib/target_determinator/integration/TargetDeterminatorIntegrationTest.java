package com.github.bazel_contrib.target_determinator.integration;

import com.github.bazel_contrib.target_determinator.label.Label;
import org.hamcrest.CoreMatchers;

import java.nio.file.Path;
import java.util.Set;

import static junit.framework.TestCase.fail;
import static org.hamcrest.MatcherAssert.assertThat;

public class TargetDeterminatorIntegrationTest extends Tests {

  @Override
  Set<Label> getTargets(Path workspace, String commitBefore, String commitAfter)
      throws TargetComputationErrorException {
    return TargetDeterminator.getTargets(
        workspace,
        "--working-directory",
        workspace.toString(),
        "--bazel",
        "bazelisk",
        "--bazel-opts",
        "--java_runtime_version=remotejdk_11",
        "--ignore-file",
        ignoredDirectoryName,
        "--delete-cached-worktree",
        commitBefore);
  }

  @Override
  protected boolean supportsIgnoredUnaddedFiles() {
    return true;
  }

  @Override
  public void refactoringStarlarkRuleIsNoOp() throws Exception {
    allowOverBuilds(
        "Rule implementation attr factors in hashes of entire transitively loaded bzl files, rather"
            + " than anything more granular or processed");
    super.refactoringStarlarkRuleIsNoOp();
  }

  @Override
  public void importInBazelrcAffectingJava() throws Exception {
    allowOverBuilds(
        "Configuration calculation doesn't appear to trim java fragments from sh_test"
            + " configuration, so Java changes are viewed to also affect sh_test targets");
    super.importInBazelrcAffectingJava();
  }

  @Override
  public void changedBazelrcAffectingSomeTests() throws Exception {
    allowOverBuilds(
        "Configuration calculation doesn't appear to trim java fragments from sh_test"
            + " configuration, so Java changes are viewed to also affect sh_test targets");
    super.changedBazelrcAffectingSomeTests();
  }

  @Override
  public void tryImportInBazelrcAffectingJava() throws Exception {
    allowOverBuilds(
        "Configuration calculation doesn't appear to trim java fragments from sh_test"
            + " configuration, so Java changes are viewed to also affect sh_test targets");
    super.tryImportInBazelrcAffectingJava();
  }

  // TODO: Stop ignoring this test when our tests use Bazel 7.0.0
  @Override
  public void addingTargetUsedInHostConfiguration() throws Exception {
    allowOverBuilds(
        "cquery doesn't factor configuration into ruleInputs, so we can't differentiate between"
            + " host and target deps. See"
            + " https://github.com/bazelbuild/bazel/issues/14610#issuecomment-1024460141");
    super.addingTargetUsedInHostConfiguration();
  }

  @Override
  public void changingHostConfigurationDoesNotAffectTargetConfiguration() throws Exception {
    allowOverBuilds(
        "cquery doesn't factor configuration into ruleInputs, so we can't differentiate between"
            + " host and target deps. See"
            + " https://github.com/bazelbuild/bazel/issues/14610#issuecomment-1024460141");
    super.changingHostConfigurationDoesNotAffectTargetConfiguration();
  }

  @Override
  public void changingTargetConfigurationDoesNotAffectHostConfiguration() throws Exception {
    allowOverBuilds(
        "cquery doesn't factor configuration into ruleInputs, so we can't differentiate between"
            + " host and target deps. See"
            + " https://github.com/bazelbuild/bazel/issues/14610#issuecomment-1024460141");
    super.changingTargetConfigurationDoesNotAffectHostConfiguration();
  }

  @Override
  public void reducingVisibilityOnDependencyAffectsTarget() throws Exception {
    // Target-determinator runs cquery, not query, which eagerly errors on invalid visibility.
    // Failing with a good error message is a reasonable alternative to passing but returning targets which are known not to be buildable.
    try {
      doTest(
          Commits.ADD_INDIRECTION_FOR_SIMPLE_JAVA_LIBRARY,
          Commits.REDUCE_DEPENDENCY_VISIBILITY,
          Set.of("//java/example:ExampleTest", "//java/example/simple"));
      fail("Expected target-determinator command to fail but it succeeded");
    } catch (TargetComputationErrorException e) {
      assertThat(e.getStdout(), CoreMatchers.equalTo("Target Determinator invocation Error\n"));
      assertThat(e.getStderr(), CoreMatchers.containsString("target '//java/example/simple:simple' is not visible from target '//java/example:ExampleTest'"));
    }
  }

  // TODO: Stop ignoring this test when our tests use Bazel 7.0.0
  @Override
  public void ignoredPlatformSpecificDepChanged() throws Exception {
    allowOverBuilds("Platform-specific narrowing is disabled due to https://github.com/bazelbuild/bazel/issues/17916");
    super.ignoredPlatformSpecificDepChanged();
  }
}
