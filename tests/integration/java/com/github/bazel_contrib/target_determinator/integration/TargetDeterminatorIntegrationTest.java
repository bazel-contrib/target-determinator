package com.github.bazel_contrib.target_determinator.integration;

import com.github.bazel_contrib.target_determinator.label.Label;
import java.io.IOException;
import java.nio.file.Path;
import java.util.Set;

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
        "--ignore-file",
        ignoredDirectoryName,
        commitBefore);
  }

  @Override
  protected boolean supportsIgnoredUnaddedFiles() {
    return true;
  }

  @Override
  public void refactoringStarlarkRuleIsNoOp() {
    allowOverBuilds(
        "Rule implementation attr factors in hashes of entire transitively loaded bzl files, rather"
            + " than anything more granular or processed");
    super.refactoringStarlarkRuleIsNoOp();
  }

  @Override
  public void importInBazelrcAffectingJava() {
    allowOverBuilds(
        "Configuration calculation doesn't appear to trim java fragments from sh_test"
            + " configuration, so Java changes are viewed to also affect sh_test targets");
    super.importInBazelrcAffectingJava();
  }

  @Override
  public void changedBazelrcAffectingSomeTests() {
    allowOverBuilds(
        "Configuration calculation doesn't appear to trim java fragments from sh_test"
            + " configuration, so Java changes are viewed to also affect sh_test targets");
    super.changedBazelrcAffectingSomeTests();
  }

  @Override
  public void tryImportInBazelrcAffectingJava() {
    allowOverBuilds(
        "Configuration calculation doesn't appear to trim java fragments from sh_test"
            + " configuration, so Java changes are viewed to also affect sh_test targets");
    super.tryImportInBazelrcAffectingJava();
  }

  @Override
  public void addingTargetUsedInHostConfiguration() {
    allowOverBuilds(
        "cquery doesn't factor configuration into ruleInputs, so we can't differentiate between"
            + " host and target deps. See"
            + " https://github.com/bazelbuild/bazel/issues/14610#issuecomment-1024460141");
    super.addingTargetUsedInHostConfiguration();
  }

  @Override
  public void changingHostConfigurationDoesNotAffectTargetConfiguration() {
    allowOverBuilds(
        "cquery doesn't factor configuration into ruleInputs, so we can't differentiate between"
            + " host and target deps. See"
            + " https://github.com/bazelbuild/bazel/issues/14610#issuecomment-1024460141");
    super.changingHostConfigurationDoesNotAffectTargetConfiguration();
  }

  @Override
  public void changingTargetConfigurationDoesNotAffectHostConfiguration() {
    allowOverBuilds(
        "cquery doesn't factor configuration into ruleInputs, so we can't differentiate between"
            + " host and target deps. See"
            + " https://github.com/bazelbuild/bazel/issues/14610#issuecomment-1024460141");
    super.changingTargetConfigurationDoesNotAffectHostConfiguration();
  }
}
