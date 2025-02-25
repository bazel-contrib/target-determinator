package com.github.bazel_contrib.target_determinator.integration;

import static junit.framework.TestCase.fail;
import static org.hamcrest.MatcherAssert.assertThat;
import static org.junit.Assume.assumeFalse;

import com.github.bazel_contrib.target_determinator.label.Label;

import java.io.IOException;
import java.io.UncheckedIOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.attribute.PosixFilePermissions;
import java.nio.file.attribute.PosixFilePermission;
import java.util.Set;

import org.junit.*;
import org.junit.rules.TemporaryFolder;
import org.junit.rules.TestName;
/**
 * Tests for target determinators.
 *
 * <p>Tests in this class should be general, applying to all reasonable possible target determinator
 * implementations, and each implementation should have its own subclass of this one.
 *
 * <p>This class will check out a copy of
 * https://github.com/bazel-contrib/target-determinator-testdata unless the system property
 * `target_determinator_testdata_dir` is set. If it is set, it should point at a clone of that
 * repository.
 */
public abstract class Tests {

  @Rule
  public TemporaryFolder tempDir = new TemporaryFolder();

  /**
   * Get the targets affected by the diff between two commits.
   *
   * @param workspace The directory of the git repository containing the changes.
   * @param commitBefore The sha1 of the commit being used as the base.
   * @param commitAfter The sha1 of the commit containing changes to be tested.
   * @return A set of fully-qualified absolute Bazel targets, of form //package/name:name. Shortened
   *     forms of labels may not be returned.
   */
  abstract Set<Label> getTargets(Path workspace, String commitBefore, String commitAfter)
      throws TargetComputationErrorException;

  protected Path testDir;

  protected static final String ignoredDirectoryName = "ignored-directory";
  protected static final String ignoredFileName = "some-file";

  private boolean allowOverBuilds = false;

  @Rule public TestName name = new TestName();

  private Set<Label> getTargets(String commitBefore, String commitAfter)
      throws TargetComputationErrorException {
    return getTargets(testDir, commitBefore, commitAfter);
  }

  protected void allowOverBuilds(String reason) {
    this.allowOverBuilds = true;
  }

  protected boolean supportsIgnoredUnstagedFiles() {
    return false;
  }

  @Before
  public void createTestRepository() throws Exception {
    System.out.println("Testing " + name.getMethodName());

    testDir = tempDir.newFolder("target-determinator-testdata_dir-clone").toPath();
  }

  @Test
  public void zeroToOneTarget_native() throws Exception {
    doTest(Commits.NO_TARGETS, Commits.ONE_TEST, Set.of("//java/example:ExampleTest"));
  }

  @Test
  public void addedTarget_native() throws Exception {
    doTest(Commits.ONE_TEST, Commits.TWO_TESTS, Set.of("//java/example:OtherExampleTest"));
  }

  @Test
  public void deletedTarget_native() throws Exception {
    doTest(
        Commits.TWO_TESTS, Commits.ONE_TEST, Set.of(), Set.of("//java/example:OtherExampleTest"));
  }

  @Test
  public void ruleAffectingAttributeChange_native() throws Exception {
    doTest(Commits.TWO_TESTS, Commits.HAS_JVM_FLAGS, Set.of("//java/example:ExampleTest"));
  }

  @Test
  public void explicitlySpecifyingDefaultValueDoesNotTrigger_native() throws Exception {
    doTest(Commits.TWO_TESTS, Commits.EXPLICIT_DEFAULT_VALUE, Set.of());
  }

  @Test
  public void changingUnimportantPermissionDoesNotTrigger_native() throws Exception {
    assumeFalse(isWindows());

    gitCheckout(Commits.EXPLICIT_DEFAULT_VALUE);
    Path srcFile = Path.of("java/example/ExampleTest.java");
    changeFileMode(srcFile, "r--r--r--");
    doTest(Commits.TWO_TESTS, Commits.EXPLICIT_DEFAULT_VALUE, Set.of());
    changeFileMode(srcFile, "rw-rw-rw-");
    doTest(Commits.TWO_TESTS, Commits.EXPLICIT_DEFAULT_VALUE, Set.of());
  }

  @Test
  public void changingImportantPermissionDoesTriggers_native() throws Exception {
    assumeFalse(isWindows());
    TestRepo repo = TestRepo.create(testDir);

    repo.replaceWithContentsFrom(Commits.TWO_TESTS);
    String beforeCommit = repo.commit("Two tests");

    repo.replaceWithContentsFrom(Commits.EXPLICIT_DEFAULT_VALUE);
    String afterCommit = repo.commit("Explicit default value");

    Path srcFile = Path.of("java/example/ExampleTest.java");
    changeFileMode(srcFile, "rwxr--r--");

    assertTargetDeterminatorRun(beforeCommit, afterCommit, Set.of("//java/example:ExampleTest"), Set.of());
  }

  @Test
  public void changedBazelMajorVersion_native() throws Exception {
    // Between Bazel 5 and 6, the attributes available on java_test changed, which means
    // these targets may be picked up as changed if a target determinator is just doing
    // hashing of target-local data.
    // However, the PatchVersion tests are also significant - changing Bazel versions may change
    // how RuleClasses are interpreted, or internal implementation details of them not captured
    // by their query-observable interface.
    doTest(
        Commits.TWO_TESTS,
        Commits.TWO_NATIVE_TESTS_BAZEL5_4_0,
        Set.of("//java/example:ExampleTest", "//java/example:OtherExampleTest"));
  }

  @Test
  public void changedBazelPatchVersion_native() throws Exception {
    doTest(
        Commits.TWO_TESTS,
        Commits.TWO_NATIVE_TESTS_BAZEL6_0_0,
        Set.of("//java/example:ExampleTest", "//java/example:OtherExampleTest"));
  }

  @Test
  public void changedBazelMajorVersion_starlark() throws Exception {
    doTest(
        Commits.SIMPLE_JAVA_LIBRARY_TARGETS,
        Commits.SIMPLE_TARGETS_BAZEL5_4_0,
        Set.of("//java/example/simple:simple", "//java/example/simple:simple_dep"));
  }

  @Test
  public void changedBazelPatchVersion_starlark() throws Exception {
    doTest(
        Commits.SIMPLE_JAVA_LIBRARY_TARGETS,
        Commits.SIMPLE_TARGETS_BAZEL6_0_0,
        Set.of("//java/example/simple:simple", "//java/example/simple:simple_dep"));
  }

  @Test
  public void changedSrc() throws Exception {
    doTest(Commits.TWO_TESTS, Commits.MODIFIED_TEST_SRC, Set.of("//java/example:ExampleTest"));
  }

  @Test
  public void changedTransitiveSrc() throws Exception {
    doTest(
        Commits.SIMPLE_JAVA_LIBRARY_TARGETS,
        Commits.CHANGE_TRANSITIVE_FILE,
        Set.of("//java/example/simple:simple", "//java/example/simple:simple_dep"));
  }

  @Test
  public void changedBazelrcAffectingAllTests() throws Exception {
    doTest(
        Commits.TWO_LANGUAGES_OF_TESTS,
        Commits.BAZELRC_TEST_ENV,
        Set.of("//java/example:ExampleTest", "//java/example:OtherExampleTest", "//sh:sh_test"));
  }

  @Test
  public void changedBazelrcAffectingSomeTests() throws Exception {
    doTest(
        Commits.TWO_LANGUAGES_OF_TESTS,
        Commits.BAZELRC_AFFECTING_JAVA,
        Set.of("//java/example:ExampleTest", "//java/example:OtherExampleTest"));
  }

  @Test
  public void emptyTryImportInBazelrc() throws Exception {
    doTest(Commits.TWO_TESTS, Commits.ADD_OPTIONAL_PRESENT_EMPTY_BAZELRC, Set.of());
  }

  @Test
  public void tryImportMissing() throws Exception {
    doTest(
        Commits.TWO_LANGUAGES_OF_TESTS,
        Commits.TWO_LANGUAGES_OPTIONAL_MISSING_TRY_IMPORT,
        Set.of());
  }

  @Test
  public void tryImportInBazelrcAffectingJava() throws Exception {
    allowOverBuilds(
        "Configuration calculation doesn't appear to trim java fragments from sh_test"
            + " configuration, so Java changes are viewed to also affect sh_test targets");
    doTest(
        Commits.TWO_LANGUAGES_OPTIONAL_MISSING_TRY_IMPORT,
        Commits.TWO_LANGUAGES_OPTIONAL_PRESENT_BAZELRC_AFFECTING_JAVA,
        Set.of("//java/example:ExampleTest", "//java/example:OtherExampleTest"));
    doTest(
        Commits.TWO_LANGUAGES_OF_TESTS,
        Commits.TWO_LANGUAGES_OPTIONAL_PRESENT_BAZELRC_AFFECTING_JAVA,
        Set.of("//java/example:ExampleTest", "//java/example:OtherExampleTest"));
    doTest(
        Commits.TWO_LANGUAGES_OPTIONAL_PRESENT_BAZELRC_AFFECTING_JAVA,
        Commits.TWO_LANGUAGES_OF_TESTS,
        Set.of("//java/example:ExampleTest", "//java/example:OtherExampleTest"));
  }

  @Test
  public void importInBazelrcNotAffectingJava() throws Exception {
    doTest(Commits.TWO_LANGUAGES_OF_TESTS, Commits.TWO_LANGUAGES_NOOP_IMPORTED_BAZELRC, Set.of());
  }

  @Test
  public void importInBazelrcAffectingJava() throws Exception {
    doTest(
        Commits.TWO_LANGUAGES_OF_TESTS,
        Commits.TWO_LANGUAGES_IMPORTED_BAZELRC_AFFECTING_JAVA,
        Set.of("//java/example:ExampleTest", "//java/example:OtherExampleTest"));
  }

  @Test
  public void addedUnusedStarlarkRulesTriggersNoTargets() throws Exception {
    doTest(Commits.TWO_TESTS, Commits.JAVA_TESTS_AND_SIMPLE_JAVA_RULES, Set.of());
  }

  @Test
  public void starlarkRulesTrigger() throws Exception {
    doTest(
        Commits.SIMPLE_JAVA_LIBRARY_RULE,
        Commits.SIMPLE_JAVA_LIBRARY_TARGETS,
        Set.of("//java/example/simple:simple", "//java/example/simple:simple_dep"));
  }

  @Test
  public void addingDepOnStarlarkRulesTrigger() throws Exception {
    doTest(
        Commits.SIMPLE_JAVA_LIBRARY_AND_JAVA_TESTS,
        Commits.DEP_ON_STARLARK_TARGET,
        Set.of("//java/example:ExampleTest"));
  }

  @Test
  public void changingStarlarkRuleDefinition() throws Exception {
    doTest(
        Commits.DEP_ON_STARLARK_TARGET,
        Commits.CHANGE_STARLARK_RULE_IMPLEMENTATION,
        Set.of(
            "//java/example:ExampleTest",
            "//java/example/simple:simple",
            "//java/example/simple:simple_dep"));
  }

  @Test
  public void refactoringStarlarkRuleIsNoOp() throws Exception {
    doTest(
        Commits.CHANGE_STARLARK_RULE_IMPLEMENTATION,
        Commits.NOOP_REFACTOR_STARLARK_RULE_IMPLEMENTATION,
        Set.of());
  }

  @Test
  public void movingStarlarkRuleToExternalRepoIsNoOp() throws Exception {
    doTest(
        Commits.NOOP_REFACTOR_STARLARK_RULE_IMPLEMENTATION,
        Commits.RULES_IN_EXTERNAL_REPO,
        Set.of());
  }

  @Test
  public void refactoringWorkspaceFileInNoOp() throws Exception {
    doTest(Commits.RULES_IN_EXTERNAL_REPO, Commits.NOOP_REFACTOR_IN_WORKSPACE_FILE, Set.of());
  }

  @Test
  public void modifyingRuleViaWorkspaceFile() throws Exception {
    doTest(
        Commits.NOOP_REFACTOR_IN_WORKSPACE_FILE,
        Commits.ADD_SIMPLE_PACKAGE_RULE,
        Set.of("//java/example/simple:simple_srcs"));
  }

  @Test
  public void unconsumedIndirectWorkspaceChangeIsNoOp() throws Exception {
    doTest(Commits.ADD_SIMPLE_PACKAGE_RULE, Commits.REFACTORED_WORKSPACE_INDIRECTLY, Set.of());
  }

//  @Test
//  public void changingMacroExpansionBasedOnFileExistence() throws Exception {
//    // Add a second target - changes the definition of the first target, so it should re-run:
//    doTest(
//        Commits.PATHOLOGICAL_RULES_SINGLE_TARGET,
//        Commits.PATHOLOGICAL_RULES_TWO_TARGETS,
//        Set.of("//weird:length_of_compute_lengths.0", "//weird:length_of_compute_lengths.2"));
//    // Revert...
//    doTest(
//        Commits.PATHOLOGICAL_RULES_TWO_TARGETS,
//        Commits.PATHOLOGICAL_RULES_SINGLE_TARGET,
//        Set.of("//weird:length_of_compute_lengths.0"));
//    // Add a third target - first target goes back to normal, so doesn't need re-testing compared to
//    // when there was just one:
//    doTest(
//        Commits.PATHOLOGICAL_RULES_SINGLE_TARGET,
//        Commits.PATHOLOGICAL_RULES_THREE_TARGETS,
//        Set.of("//weird:length_of_compute_lengths.2", "//weird:length_of_compute_lengths.3"));
//    // Add targets 4 and 5 - the previous rules no longer exist, but a new one does.
//    doTest(
//        Commits.PATHOLOGICAL_RULES_SINGLE_TARGET,
//        Commits.PATHOLOGICAL_RULES_FIVE_TARGETS,
//        Set.of("//weird:pathological"));
//  }

  @Test
  public void changingFileLoadedByWorkspaceTriggersTargets() throws Exception {
    doTest(
        Commits.ADD_SIMPLE_PACKAGE_RULE,
        Commits.CHANGE_ATTRIBUTES_VIA_INDIRECTION,
        Set.of(
            "//java/example:ExampleTest",
            "//java/example/simple:simple",
            "//java/example/simple:simple_dep"));
  }

  @Test
  public void removingGlobbedFileTriggers() throws Exception {
    doTest(Commits.HAS_GLOBS, Commits.CHANGE_GLOBS, Set.of("//globs:root"));
  }

  @Test
  public void removingBuildFileRetriggersGlobs() throws Exception {
    doTest(
        Commits.ADD_BUILD_FILE_INTERFERING_WTH_GLOBS, Commits.CHANGE_GLOBS, Set.of("//globs:root"));
  }

  @Test
  public void addingBuildFileRetriggersGlobs() throws Exception {
    doTest(
        Commits.CHANGE_GLOBS, Commits.ADD_BUILD_FILE_INTERFERING_WTH_GLOBS, Set.of("//globs:root"));
  }

  @Test
  public void addingTargetUsedInHostConfiguration() throws Exception {
    doTest(
        Commits.BAZELRC_INCLUDED_EMPTY,
        Commits.JAVA_USED_IN_GENRULE,
        Set.of("//configurations:jbin", "//configurations:run_jbin"));
  }

  @Test
  public void changingHostConfigurationDoesNotAffectTargetConfiguration() throws Exception {
    // Only run_jbin should be present because it's the only host java target
    doTest(
        Commits.JAVA_USED_IN_GENRULE,
        Commits.BAZELRC_HOST_JAVACOPT,
        Set.of("//configurations:run_jbin"));
  }

  @Test
  public void changingTargetConfigurationDoesNotAffectHostConfiguration() throws Exception {
    // run_jbin should not be present because it's configured in host not target
    doTest(
        Commits.JAVA_USED_IN_GENRULE,
        Commits.BAZELRC_INCLUDED_JAVACOPT,
        Set.of("//configurations:jbin", "//java/example:ExampleTest"));
  }

  @Test
  public void reducingVisibilityOnDependencyAffectsTarget() throws Exception {
    doTest(
        Commits.ADD_INDIRECTION_FOR_SIMPLE_JAVA_LIBRARY,
        Commits.REDUCE_DEPENDENCY_VISIBILITY,
        Set.of("//java/example:ExampleTest", "//java/example/simple"));
  }

  @Test
  public void succeedForUncleanRepository() throws Exception {
    Files.createFile(testDir.resolve("untracked-file"));

    doTest(Commits.TWO_TESTS, Commits.HAS_JVM_FLAGS, Set.of("//java/example:ExampleTest"));
  }

//  @Test
//  public void succeedForUncleanIgnoredFiles() throws Exception {
//    Path ignoredFile = testDir.resolve("ignored-file");
//    Files.createFile(ignoredFile);
//
//    doTest(
//        Commits.ONE_TEST,
//        Commits.TWO_TESTS_WITH_GITIGNORE,
//        Set.of("//java/example:OtherExampleTest"));
//    assertThat(
//        "expected ignored file to still be present after invocation",
//        ignoredFile.toFile().exists());
//  }
//
//  @Test
//  public void succeedForUncleanSubmodule() throws Exception {
//    gitCheckout(Commits.SUBMODULE_CHANGE_DIRECTORY);
//
//    Files.createFile(testDir.resolve("demo-submodule-2").resolve("untracked-file"));
//
//    doTest(Commits.SUBMODULE_ADD_DEPENDENT_ON_SIMPLE_JAVA_LIBRARY,
//            Commits.SUBMODULE_CHANGE_DIRECTORY,
//            Set.of("//demo-submodule-2:submodule_simple"));
//  }
//
//  @Test
//  public void addTrivialSubmodule() throws Exception {
//    doTest(Commits.SIMPLE_JAVA_LIBRARY_TARGETS, Commits.SUBMODULE_ADD_TRIVIAL_SUBMODULE, Set.of());
//    assertThat(
//        "The submodule should now be present with its README.md but isn't",
//        Files.exists(testDir.resolve("demo-submodule").resolve("README.md")));
//  }
//
//  @Test
//  public void addDependentTargetInSubmodule() throws Exception {
//    doTest(
//        Commits.SUBMODULE_ADD_TRIVIAL_SUBMODULE,
//        Commits.SUBMODULE_ADD_DEPENDENT_ON_SIMPLE_JAVA_LIBRARY,
//        Set.of("//demo-submodule:submodule_simple"));
//  }
//
//  @Test
//  public void changeSubmodulePath() throws Exception {
//    doTest(
//        Commits.SUBMODULE_ADD_DEPENDENT_ON_SIMPLE_JAVA_LIBRARY,
//        Commits.SUBMODULE_CHANGE_DIRECTORY,
//        Set.of("//demo-submodule-2:submodule_simple"));
//
//    assertThat(
//        "The old submodule directory should not exist anymore",
//        not(Files.exists(testDir.resolve("demo-submodule"))));
//
//    assertThat(
//        "The moved submodule should now be present with its README.md but isn't",
//        Files.exists(testDir.resolve("demo-submodule-2").resolve("README.md")));
//  }
//
//  @Test
//  public void deleteSubmodule() throws Exception {
//    doTest(Commits.SUBMODULE_CHANGE_DIRECTORY, Commits.SUBMODULE_DELETE_SUBMODULE, Set.of());
//
//    assertThat(
//        "The old submodule directory should not exist anymore",
//        not(Files.exists(testDir.resolve("demo-submodule-2"))));
//  }
//
  @Test
  public void testRelativeRevisions() throws Exception {
    TestRepo repo = TestRepo.create(testDir);
    repo.replaceWithContentsFrom(Commits.ONE_TEST);
    repo.commit("Before commit");

    repo.replaceWithContentsFrom(Commits.TWO_TESTS);
    repo.commit("After commit");

    doTest("HEAD^", "HEAD", Set.of("//java/example:OtherExampleTest"));
  }
//
//  @Test
//  public void testBranchRevision() throws Exception {
//    gitCheckout(Commits.TWO_TESTS);
//    gitCheckoutBranch(Commits.TWO_TESTS_BRANCH);
//    doTest(Commits.ONE_TEST, Commits.TWO_TESTS_BRANCH, Set.of("//java/example:OtherExampleTest"));
//    assertEquals(
//        "Initial branch should be checked out after running the target determinator",
//        Commits.TWO_TESTS_BRANCH,
//        gitBranch());
//  }
//
  @Test
  public void testMinimumSupportedBazelVersion() throws Exception {
    doTest(
        Commits.SIMPLE_JAVA_LIBRARY_TARGETS,
        Commits.CHANGE_TRANSITIVE_FILE_BAZEL4_0_0,
        Set.of("//java/example/simple:simple", "//java/example/simple:simple_dep"));
  }

  @Test
  public void testChmodFile() throws TargetComputationErrorException {
    doTest(Commits.ONE_SH_TEST, Commits.SH_TEST_NOT_EXECUTABLE, Set.of("//sh:sh_test"));
  }

  @Test
  public void incompatibleTargetsAreFiltered() throws Exception {
    doTest(Commits.ONE_TEST, Commits.INCOMPATIBLE_TARGET,
        Set.of("//java/example:CompatibleTest"),
        Set.of("//java/example:IncompatibleTest"));
  }

  @Test
  public void incompatibleTargetsAreFiltered_bazelIssue21010() throws Exception {
    doTest(Commits.ONE_TEST_BAZEL7_0_0, Commits.INCOMPATIBLE_TARGET_BAZEL7_0_0,
        Set.of("//java/example:CompatibleTest"),
        Set.of("//java/example:IncompatibleTest"));
  }

  @Test
  public void platformSpecificSrcChanged() throws Exception {
    String after = Commits.CHANGED_NONLINUX_SRC;
    if (isLinux()) {
      after = Commits.CHANGED_LINUX_SRC;
    }

    doTest(Commits.SELECT_TARGET, after, Set.of("//java/example/simple:simple"));
  }

  @Test
  public void ignoredPlatformSpecificSrcChanged() throws Exception {
    String after = Commits.CHANGED_LINUX_SRC;
    if (isLinux()) {
      after = Commits.CHANGED_NONLINUX_SRC;
    }

    doTest(Commits.SELECT_TARGET, after, Set.of());
  }

  @Test
  public void platformSpecificDepChanged() throws Exception {
    String after = Commits.CHANGED_NONLINUX_DEP;
    String changedDepTarget = "//java/example/simple:simple_dep";
    if (isLinux()) {
      after = Commits.CHANGED_LINUX_DEP;
      changedDepTarget = "//java/example/simple:simple_dep_linux";
    }

    doTest(Commits.SELECT_TARGET, after, Set.of("//java/example/simple:simple", changedDepTarget));
  }

  @Test
  public void ignoredPlatformSpecificDepChanged() throws Exception {
    String after = Commits.CHANGED_LINUX_DEP;
    String changedDepTarget = "//java/example/simple:simple_dep_linux";
    if (isLinux()) {
      after = Commits.CHANGED_NONLINUX_DEP;
      changedDepTarget = "//java/example/simple:simple_dep";
    }

    doTest(Commits.SELECT_TARGET, after, Set.of(changedDepTarget));
  }

  @Test
  public void aliasTargetIsDetectedIfActualTargetChanged() throws Exception {
    doTest(Commits.ALIAS_ADD_TARGET, Commits.ALIAS_CHANGE_TARGET_THROUGH_ALIAS,
            Set.of("//java/example:ExampleTest", "//java/example:example_test"));
  }

  @Test
  public void aliasTargetIsDetectedIfActualFileChanged() throws Exception {
    doTest(Commits.ALIAS_ADD_TARGET_TO_FILE, Commits.ALIAS_CHANGE_TARGET_THROUGH_ALIAS_TO_FILE,
            Set.of("//java/example:ExampleTest", "//java/example:ExampleTestSource"));
  }

  @Test
  public void aliasTargetIsDetectedIfActualLabelChanged() throws Exception {
    doTest(Commits.ALIAS_ADD_TARGET, Commits.ALIAS_CHANGE_ACTUAL,
            Set.of("//java/example:example_test"));
  }

  public void doTest(
          String beforeCommit,
          String afterCommit,
          Set<String> expectedTargets) throws TargetComputationErrorException {
    doTest(beforeCommit, afterCommit, expectedTargets, Set.of());
  }

  public void doTest(
          String beforeCommit,
          String afterCommit,
          Set<String> expectedTargets,
          Set<String> forbiddenTargetStrings) throws TargetComputationErrorException {
    TestRepo repo = TestRepo.create(testDir);
    repo.replaceWithContentsFrom(beforeCommit);
    String commitBefore = repo.commit("Before commit");

    repo.replaceWithContentsFrom(afterCommit);
    String commitAfter = repo.commit("After commit");

    assertTargetDeterminatorRun(commitBefore, commitAfter, expectedTargets, forbiddenTargetStrings);
  }

  private void assertTargetDeterminatorRun(
          String commitBefore,
          String commitAfter,
          Set<String> expectedTargets,
          Set<String> forbiddenTargetStrings) throws TargetComputationErrorException {
    if (supportsIgnoredUnstagedFiles()) {
      try {
        Path ignoredDirectory = testDir.resolve(ignoredDirectoryName);
        Files.createDirectory(ignoredDirectory);
        Files.createFile(ignoredDirectory.resolve(ignoredFileName));
      } catch (IOException e) {
        throw new UncheckedIOException(e);
      }
    }

    Set<Label> targets = getTargets(commitBefore, commitAfter);
    Util.assertTargetsMatch(
            targets, expectedTargets, forbiddenTargetStrings, allowOverBuilds);

    if (supportsIgnoredUnstagedFiles()) {
      assertThat(
              "Ignored files should still be around after running the target determination executable"
                      + " but wasn't",
              testDir.resolve(ignoredDirectoryName).resolve(ignoredFileName).toFile().exists());
    }
  }

  private void gitCheckoutBranch(String branch) throws Exception {
    TestdataRepo.gitCheckoutBranch(testDir, branch);
  }

  private void gitCheckout(String commitName) throws Exception {
    TestRepo repo = TestRepo.create(testDir);
    repo.replaceWithContentsFrom(commitName);
  }

  private String gitBranch() throws Exception {
    return TestdataRepo.gitBranch(testDir);
  }

  private boolean isLinux() {
    return "Linux".equals(System.getProperty("os.name"));
  }

  private boolean isWindows() {
    return System.getProperty("os.name").startsWith("Windows");
  }

  private void changeFileMode(final Path filePath, final String fileModeString) {
    Set<PosixFilePermission> filePermissions = PosixFilePermissions.fromString(fileModeString);
    try {
      Files.setPosixFilePermissions(testDir.resolve(filePath), filePermissions);
    } catch (Exception e) {
      fail(e.getMessage());
    }
  }
}
