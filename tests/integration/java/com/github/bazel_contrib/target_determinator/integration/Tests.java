package com.github.bazel_contrib.target_determinator.integration;

import static junit.framework.TestCase.assertEquals;
import static junit.framework.TestCase.fail;
import static org.hamcrest.MatcherAssert.assertThat;
import static org.hamcrest.Matchers.not;

import com.github.bazel_contrib.target_determinator.label.Label;

import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.attribute.PosixFilePermissions;
import java.nio.file.attribute.PosixFilePermission;
import java.util.Set;
import org.eclipse.jgit.util.FileUtils;
import org.junit.*;
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

  protected static TestdataRepo testdataRepo;

  // Contains a new clone of the testdata repository each time a test is run.
  // Should not change its path between builds, to avoid having to clean-start bazel for each test,
  // but is cleaned between tests.
  protected static Path testDir;

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

  protected boolean supportsIgnoredUnaddedFiles() {
    return false;
  }

  @BeforeClass
  public static void cloneRepo() throws Exception {
    testdataRepo = Util.cloneTestdataRepo();
    testDir = Files.createTempDirectory("targe-determinator-testdata-dir-clone");
  }

  @Before
  public void createTestRepository() throws Exception {
    System.out.println("Testing " + name.getMethodName());
    // Create a clean, bare repository to ensure that the checkout will be pristine.
    testdataRepo.cloneTo(testDir);
    Path ignoredDirectory = testDir.resolve(ignoredDirectoryName);
    if (supportsIgnoredUnaddedFiles()) {
      Files.createDirectory(ignoredDirectory);
      Files.createFile(ignoredDirectory.resolve(ignoredFileName));
    }
  }

  @After
  public void cleanupTestRepository() throws Exception {
    FileUtils.delete(testDir.toFile(), FileUtils.RECURSIVE | FileUtils.SKIP_MISSING);
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
    if (!isWindows()) {
      gitCheckout(Commits.EXPLICIT_DEFAULT_VALUE);
      Path srcFile = Path.of("java/example/ExampleTest.java");
      changeFileMode(srcFile, "r--r--r--");
      doTest(Commits.TWO_TESTS, Commits.EXPLICIT_DEFAULT_VALUE, Set.of());
      changeFileMode(srcFile, "rw-rw-rw-");
      doTest(Commits.TWO_TESTS, Commits.EXPLICIT_DEFAULT_VALUE, Set.of());
    }
  }

  @Test
  public void changingImportantPermissionDoesTriggers_native() throws Exception {
    if (!isWindows()) {
      gitCheckout(Commits.EXPLICIT_DEFAULT_VALUE);
      Path srcFile = Path.of("java/example/ExampleTest.java");
      changeFileMode(srcFile, "rwxr--r--");
      doTest(Commits.TWO_TESTS, Commits.EXPLICIT_DEFAULT_VALUE, Set.of("//java/example:ExampleTest"));
    }
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

  @Test
  public void changingMacroExpansionBasedOnFileExistence() throws Exception {
    // Add a second target - changes the definition of the first target, so it should re-run:
    doTest(
        Commits.PATHOLOGICAL_RULES_SINGLE_TARGET,
        Commits.PATHOLOGICAL_RULES_TWO_TARGETS,
        Set.of("//weird:length_of_compute_lengths.0", "//weird:length_of_compute_lengths.2"));
    // Revert...
    doTest(
        Commits.PATHOLOGICAL_RULES_TWO_TARGETS,
        Commits.PATHOLOGICAL_RULES_SINGLE_TARGET,
        Set.of("//weird:length_of_compute_lengths.0"));
    // Add a third target - first target goes back to normal, so doesn't need re-testing compared to
    // when there was just one:
    doTest(
        Commits.PATHOLOGICAL_RULES_SINGLE_TARGET,
        Commits.PATHOLOGICAL_RULES_THREE_TARGETS,
        Set.of("//weird:length_of_compute_lengths.2", "//weird:length_of_compute_lengths.3"));
    // Add targets 4 and 5 - the previous rules no longer exist, but a new one does.
    doTest(
        Commits.PATHOLOGICAL_RULES_SINGLE_TARGET,
        Commits.PATHOLOGICAL_RULES_FIVE_TARGETS,
        Set.of("//weird:pathological"));
  }

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


  @Test
  public void succeedForUncleanIgnoredFiles() throws Exception {
    Path ignoredFile = testDir.resolve("ignored-file");
    Files.createFile(ignoredFile);

    doTest(
        Commits.ONE_TEST,
        Commits.TWO_TESTS_WITH_GITIGNORE,
        Set.of("//java/example:OtherExampleTest"));
    assertThat(
        "expected ignored file to still be present after invocation",
        ignoredFile.toFile().exists());
  }

  @Test
  public void succeedForUncleanSubmodule() throws Exception {
    gitCheckout(Commits.SUBMODULE_CHANGE_DIRECTORY);

    Files.createFile(testDir.resolve("demo-submodule-2").resolve("untracked-file"));

    doTest(Commits.SUBMODULE_ADD_DEPENDENT_ON_SIMPLE_JAVA_LIBRARY,
            Commits.SUBMODULE_CHANGE_DIRECTORY,
            Set.of("//demo-submodule-2:submodule_simple"));
  }

  @Test
  public void addTrivialSubmodule() throws Exception {
    doTest(Commits.SIMPLE_JAVA_LIBRARY_TARGETS, Commits.SUBMODULE_ADD_TRIVIAL_SUBMODULE, Set.of());
    assertThat(
        "The submodule should now be present with its README.md but isn't",
        Files.exists(testDir.resolve("demo-submodule").resolve("README.md")));
  }

  @Test
  public void addDependentTargetInSubmodule() throws Exception {
    doTest(
        Commits.SUBMODULE_ADD_TRIVIAL_SUBMODULE,
        Commits.SUBMODULE_ADD_DEPENDENT_ON_SIMPLE_JAVA_LIBRARY,
        Set.of("//demo-submodule:submodule_simple"));
  }

  @Test
  public void changeSubmodulePath() throws Exception {
    doTest(
        Commits.SUBMODULE_ADD_DEPENDENT_ON_SIMPLE_JAVA_LIBRARY,
        Commits.SUBMODULE_CHANGE_DIRECTORY,
        Set.of("//demo-submodule-2:submodule_simple"));

    assertThat(
        "The old submodule directory should not exist anymore",
        not(Files.exists(testDir.resolve("demo-submodule"))));

    assertThat(
        "The moved submodule should now be present with its README.md but isn't",
        Files.exists(testDir.resolve("demo-submodule-2").resolve("README.md")));
  }

  @Test
  public void deleteSubmodule() throws Exception {
    doTest(Commits.SUBMODULE_CHANGE_DIRECTORY, Commits.SUBMODULE_DELETE_SUBMODULE, Set.of());

    assertThat(
        "The old submodule directory should not exist anymore",
        not(Files.exists(testDir.resolve("demo-submodule-2"))));
  }

  @Test
  public void testRelativeRevisions() throws Exception {
    gitCheckout(Commits.TWO_TESTS);
    doTest("HEAD^", "HEAD", Set.of("//java/example:OtherExampleTest"));
  }

  @Test
  public void testBranchRevision() throws Exception {
    gitCheckout(Commits.TWO_TESTS);
    final String twoTestsBranch = "two-tests-branch";
    gitCheckoutBranch(twoTestsBranch);
    doTest(Commits.ONE_TEST, twoTestsBranch, Set.of("//java/example:OtherExampleTest"));
    assertEquals(
        "Initial branch should be checked out after running the target determinator",
        twoTestsBranch,
        gitBranch());
  }

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

  public void doTest(String commitBefore, String commitAfter, Set<String> expectedTargets) throws TargetComputationErrorException {
    doTest(commitBefore, commitAfter, expectedTargets, Set.of());
  }

  public void doTest(
      String commitBefore,
      String commitAfter,
      Set<String> expectedTargetStrings,
      Set<String> forbiddenTargetStrings) throws TargetComputationErrorException {
    // Check out the commitAfter as it is a requirement for target-determinator.
    try {
      gitCheckout(commitAfter);
    } catch (Exception e) {
      fail(e.getMessage());
    }

    Set<Label> targets = getTargets(commitBefore, commitAfter);
    Util.assertTargetsMatch(
        targets, expectedTargetStrings, forbiddenTargetStrings, allowOverBuilds);

    if (supportsIgnoredUnaddedFiles()) {
      assertThat(
          "Ignored files should still be around after running the target determination executable"
              + " but wasn't",
          testDir.resolve(ignoredDirectoryName).resolve(ignoredFileName).toFile().exists());
    }
  }

  private void gitCheckoutBranch(final String branch) throws Exception {
    TestdataRepo.gitCheckoutBranch(testDir, branch);
  }

  private void gitCheckout(final String commit) throws Exception {
    TestdataRepo.gitCheckout(testDir, commit);
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

class Commits {

  public static final String NO_TARGETS = "v1/new-branch";
  public static final String ONE_TEST = "v1/one-test";
  public static final String ONE_TEST_BAZEL7_0_0 = "v1/one-test-bazel-7.0.0";
  public static final String TWO_TESTS = "v1/two-tests";
  public static final String HAS_JVM_FLAGS = "v1/has-jvm-flags";
  public static final String EXPLICIT_DEFAULT_VALUE = "v1/explicit-default-value";
  public static final String TWO_NATIVE_TESTS_BAZEL5_4_0 = "v1/two-native-tests-bazel-5.4.0";
  public static final String TWO_NATIVE_TESTS_BAZEL6_0_0 = "v1/two-native-tests-bazel-6.0.0";
  public static final String MODIFIED_TEST_SRC = "v1/modified-test-src";
  public static final String TWO_LANGUAGES_OF_TESTS = "v1/two-languages-of-tests";
  public static final String BAZELRC_TEST_ENV = "v1/bazelrc-test-env";
  public static final String BAZELRC_AFFECTING_JAVA = "v1/bazelrc-affecting-java";
  public static final String SIMPLE_TARGETS_BAZEL5_4_0 = "v1/simple-targets-bazel-5.4.0";
  public static final String SIMPLE_TARGETS_BAZEL6_0_0 = "v1/simple-targets-bazel-6.0.0";
  public static final String ADD_OPTIONAL_PRESENT_EMPTY_BAZELRC = "v1/optional-present-empty-try-import";
  public static final String SIMPLE_JAVA_LIBRARY_RULE = "v1/simple-java-library-rule";
  public static final String SIMPLE_JAVA_LIBRARY_TARGETS = "v1/simple-java-library-targets";
  public static final String SIMPLE_JAVA_LIBRARY_AND_JAVA_TESTS = "v1/simple-java-library-and-java-tests";
  public static final String CHANGE_TRANSITIVE_FILE = "v1/change-transitive-file";
  public static final String CHANGE_TRANSITIVE_FILE_BAZEL4_0_0 = "v1/change-transitive-file-bazel-4.0.0";
  public static final String TWO_LANGUAGES_OPTIONAL_MISSING_TRY_IMPORT = "v1/two-languages-missing-try-import";
  public static final String TWO_LANGUAGES_OPTIONAL_PRESENT_BAZELRC_AFFECTING_JAVA = "v1/two-languages-optional-present-bazelrc-affecting-java";
  public static final String TWO_LANGUAGES_NOOP_IMPORTED_BAZELRC = "v1/two-languages-noop-imported-bazelrc";
  public static final String TWO_LANGUAGES_IMPORTED_BAZELRC_AFFECTING_JAVA = "v1/two-languages-imported-bazelrc-affecting-java";
  public static final String JAVA_TESTS_AND_SIMPLE_JAVA_RULES = "v1/java-tests-and-simple-java-library-rule";
  public static final String DEP_ON_STARLARK_TARGET = "v1/dep-on-starlark-target";
  public static final String CHANGE_STARLARK_RULE_IMPLEMENTATION = "v1/change-starlark-rule-implementation";
  public static final String NOOP_REFACTOR_STARLARK_RULE_IMPLEMENTATION = "v1/noop-refactor-starlark-rule-implementation";
  public static final String RULES_IN_EXTERNAL_REPO = "v1/move-rules-to-external-repo";
  public static final String NOOP_REFACTOR_IN_WORKSPACE_FILE = "v1/noop-refactor-in-workspace-file";
  public static final String ADD_SIMPLE_PACKAGE_RULE = "v1/add-simple-package-rule";
  public static final String REFACTORED_WORKSPACE_INDIRECTLY = "v1/refactored-workspace-indirectly";
  public static final String PATHOLOGICAL_RULES_SINGLE_TARGET = "v1/pathological-rules-single-target";
  public static final String PATHOLOGICAL_RULES_TWO_TARGETS = "v1/pathological-rules-two-targets";
  public static final String PATHOLOGICAL_RULES_THREE_TARGETS = "v1/pathological-rules-three-targets";
  public static final String PATHOLOGICAL_RULES_FIVE_TARGETS = "v1/pathological-rules-five-targets";
  public static final String CHANGE_ATTRIBUTES_VIA_INDIRECTION = "v1/set-flags-via-indirected-rules";
  public static final String HAS_GLOBS = "v1/globs";
  public static final String CHANGE_GLOBS = "v1/globs-changed";
  public static final String ADD_BUILD_FILE_INTERFERING_WTH_GLOBS = "v1/globs-add-interfering-build-file";
  public static final String BAZELRC_INCLUDED_EMPTY = "v1/bazelrc-included-empty";
  public static final String JAVA_USED_IN_GENRULE = "v1/java-used-in-genrule";
  public static final String BAZELRC_INCLUDED_JAVACOPT = "v1/bazelrc-included-javacopt";
  public static final String BAZELRC_HOST_JAVACOPT = "v1/bazelrc-host-javacopt";
  public static final String ADD_INDIRECTION_FOR_SIMPLE_JAVA_LIBRARY = "v1/add-indirection-for-simple-java-library";
  public static final String REDUCE_DEPENDENCY_VISIBILITY = "v1/reduce-dependency-visibility";
  public static final String ONE_TEST_WITH_GITIGNORE = "v1/one-test-with-gitignore";
  public static final String TWO_TESTS_WITH_GITIGNORE = "v1/two-tests-with-gitignore";
  public static final String SUBMODULE_ADD_TRIVIAL_SUBMODULE = "v1/submodule-add-trivial-submodule";
  public static final String SUBMODULE_ADD_DEPENDENT_ON_SIMPLE_JAVA_LIBRARY = "v1/submodule-add-dependent-of-simple_java_library";
  public static final String SUBMODULE_CHANGE_DIRECTORY = "v1/submodule-change-directory";
  public static final String SUBMODULE_DELETE_SUBMODULE = "v1/submodule-delete-submodule";
  public static final String ONE_SH_TEST = "v1/sh-test"; // Add an executable shell file and BUILD.bazel file
  public static final String SH_TEST_NOT_EXECUTABLE = "v1/sh-test-non-executable"; // Make shell test file non-executable
  public static final String INCOMPATIBLE_TARGET = "v1/incompatible-target";
  public static final String INCOMPATIBLE_TARGET_BAZEL7_0_0 = "v1/incompatible-target-bazel-7.0.0";
  public static final String SELECT_TARGET = "v1/platform-specific-selects";
  public static final String CHANGED_NONLINUX_SRC = "v1/platform-specific-selects-change-non-linux-src";
  public static final String CHANGED_LINUX_SRC = "v1/platform-specific-selects-change-linux-src";
  public static final String CHANGED_NONLINUX_DEP = "v1/platform-specific-selects-change-non-linux-dep";
  public static final String CHANGED_LINUX_DEP = "v1/platform-specific-selects-change-linux-dep";
  public static final String ALIAS_ADD_TARGET = "v1/alias-add-target";
  public static final String ALIAS_CHANGE_ACTUAL = "v1/alias-change-actual";
  public static final String ALIAS_CHANGE_TARGET_THROUGH_ALIAS = "v1/alias-change-target-through-alias";
  public static final String ALIAS_ADD_TARGET_TO_FILE = "v1/alias-file-add-target";
  public static final String ALIAS_CHANGE_TARGET_THROUGH_ALIAS_TO_FILE = "v1/alias-file-change-actual";

}
