package com.github.bazel_contrib.target_determinator.integration;

import static junit.framework.TestCase.assertEquals;
import static junit.framework.TestCase.fail;
import static org.hamcrest.MatcherAssert.assertThat;
import static org.hamcrest.Matchers.not;

import com.github.bazel_contrib.target_determinator.label.Label;

import java.nio.file.Files;
import java.nio.file.Path;
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
    gitCheckoutBranch(Commits.TWO_TESTS_BRANCH);
    doTest(Commits.ONE_TEST, Commits.TWO_TESTS_BRANCH, Set.of("//java/example:OtherExampleTest"));
    assertEquals(
        "Initial branch should be checked out after running the target determinator",
        Commits.TWO_TESTS_BRANCH,
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
}

class Commits {

  public static final String NO_TARGETS = "d2862de5e63c8be0866056e6307049c159fb9e47";
  public static final String ONE_TEST = "65dfed228e75a7f4ad361fe65512a1e58ef83b1c";
  public static final String TWO_TESTS = "bd1f7781e0d5ee66f3235a1adb8f656d5ea35c2d";
  public static final String HAS_JVM_FLAGS = "50609b7d1260b449ceed57718165981986880d97";
  public static final String EXPLICIT_DEFAULT_VALUE = "34213eb339cbb5d1544c83c1aa8c19528c147e0d";
  public static final String TWO_NATIVE_TESTS_BAZEL5_4_0 = "97637aedbfdf0be9c9d440c56ddc10c842fd9e4a";
  public static final String TWO_NATIVE_TESTS_BAZEL6_0_0 = "e82404bbedebb800fed8053dfc4f2ebdbcdebcd6";
  public static final String MODIFIED_TEST_SRC = "4a0e589ac8d0d33e8e6109b07d0d60a833261eb3";
  public static final String TWO_LANGUAGES_OF_TESTS = "805a14f65edd9e3d42b6ec8524397a269065df49";
  public static final String BAZELRC_TEST_ENV = "9afe362266b9a7cd0d9dd63d16bdf9849db71199";
  public static final String BAZELRC_AFFECTING_JAVA = "b1f9504dcba4e0fdc2cf344048307fdd7ac9baec";
  public static final String SIMPLE_TARGETS_BAZEL4_0_0 = "877b2f679e65595e895a1356994344bf4b4ce45f";
  public static final String SIMPLE_TARGETS_BAZEL5_4_0 = "7cc16e4080aa7b20fa80c2e4e1dacb353ef09275";
  public static final String SIMPLE_TARGETS_BAZEL6_0_0 = "7efcdd39046bd3d0fdd9c9ab34259ce8894c5cfb";
  public static final String ADD_OPTIONAL_PRESENT_EMPTY_BAZELRC =
      "50f6d42a9fa62760ec0e2bb22a51a5e68ed87813";
  public static final String SIMPLE_JAVA_LIBRARY_RULE = "3ced8e757bbdb853553c62754ad68bce3be9033f";
  public static final String SIMPLE_JAVA_LIBRARY_TARGETS =
      "a96d8a14615e972f6a833ba70bb0a9a806e781e0";
  public static final String SIMPLE_JAVA_LIBRARY_AND_JAVA_TESTS =
      "a82f3ba70787617a78c60c2c460bf954c30be4a0";
  public static final String CHANGE_TRANSITIVE_FILE = "e93ab7f1081c2d25b54e325f402875230cb37bd7";
  public static final String CHANGE_TRANSITIVE_FILE_BAZEL4_0_0 = "c90dce5b2b6c888ba08b8cdac5eb60b031ff447f";
  public static final String TWO_LANGUAGES_OPTIONAL_MISSING_TRY_IMPORT =
      "69ed4974b6cccb990415537ffe19cc59c9b22306";
  public static final String TWO_LANGUAGES_OPTIONAL_PRESENT_BAZELRC_AFFECTING_JAVA =
      "b92b6f07812f6d440c515280d344b491614f3c6b";
  public static final String TWO_LANGUAGES_NOOP_IMPORTED_BAZELRC =
      "ebecc480402cf271f258afaa533cf36a305145b8";
  public static final String TWO_LANGUAGES_IMPORTED_BAZELRC_AFFECTING_JAVA =
      "5f8a5eafa64838d18e66c3a2977fd72c9a81f7f5";
  public static final String JAVA_TESTS_AND_SIMPLE_JAVA_RULES =
      "92d3c3c260a7c856b59e33df40f55dcfa40f04f6";
  public static final String DEP_ON_STARLARK_TARGET = "af5c807d6254150c82a33f36fce21c5ced4f50ff";
  public static final String CHANGE_STARLARK_RULE_IMPLEMENTATION =
      "f9ef8e3ad134d42b4d7391e8f02179176971a47d";
  public static final String NOOP_REFACTOR_STARLARK_RULE_IMPLEMENTATION =
      "8aa249993b0263ea4adf83d6b0cd851b711baf56";
  public static final String RULES_IN_EXTERNAL_REPO = "41a8a5272e98f8feb26f73327a87f06ca19404a1";
  public static final String NOOP_REFACTOR_IN_WORKSPACE_FILE =
      "f4325420d0b011a841a60a2c612ef4997aa5359b";
  public static final String ADD_SIMPLE_PACKAGE_RULE = "52609340c87c6cee9d6e3ac26564a46ff9a6c17a";
  public static final String REFACTORED_WORKSPACE_INDIRECTLY =
      "b3d8d9c109f1fc003ce5744961dac58773c2c71b";
  public static final String PATHOLOGICAL_RULES_SINGLE_TARGET =
      "d3fa4261ba55826781c33a1e6814de7effa8f48f";
  public static final String PATHOLOGICAL_RULES_TWO_TARGETS =
      "436d3472cd7f3c8a73cb23e0d85e86aa2eac0e0d";
  public static final String PATHOLOGICAL_RULES_THREE_TARGETS =
      "5987e2a10abef4087ae472e6171cda506190ca95";
  public static final String PATHOLOGICAL_RULES_FIVE_TARGETS =
      "8e5e5b4b1ac7eaf02ddafe9f551a1f6eda5b2191";
  public static final String CHANGE_ATTRIBUTES_VIA_INDIRECTION =
      "f4dfccca871e962bd4fa52f1d55e5169192b8343";
  public static final String HAS_GLOBS = "018f7c0b96891ca644a05ae15d8d21be020b4355";
  public static final String CHANGE_GLOBS = "307612ea08fc732e41815c4b24dfbdb47d741955";
  public static final String ADD_BUILD_FILE_INTERFERING_WTH_GLOBS =
      "8fbfba87540b48bb5e4b91a62180d4d5c6d6678e";
  public static final String BAZELRC_INCLUDED_EMPTY = "89f1396b3341c038536ab7c17942b0c5a35515bc";
  public static final String JAVA_USED_IN_GENRULE = "b941205e5a12ff6c5ae6305404b7dfe0a2e407c9";
  public static final String BAZELRC_INCLUDED_JAVACOPT = "20f1af740abde1eea14af3668d8ffb2102bfcf06";
  public static final String BAZELRC_HOST_JAVACOPT = "23948e3f11a51d3e7dc45c46853ae0a15cac6abf";
  public static final String ADD_INDIRECTION_FOR_SIMPLE_JAVA_LIBRARY =
      "c0ef0f9805e65817299eb7a794ed66655c0dd5aa";
  public static final String REDUCE_DEPENDENCY_VISIBILITY =
      "396dae111684b893ec6e04b2f6e86ed603a01082";
  public static final String TWO_TESTS_WITH_GITIGNORE = "55845a3a08525f2aa66c3d7a2115dad684c46995";
  public static final String SUBMODULE_ADD_TRIVIAL_SUBMODULE =
      "b88ddcfe3da63c8308ce6d3274dd424d2c7b211a";
  public static final String SUBMODULE_ADD_DEPENDENT_ON_SIMPLE_JAVA_LIBRARY =
      "4e9b396b3a8030925d7b544cda3f1edbc199810f";
  public static final String SUBMODULE_CHANGE_DIRECTORY =
      "1cef87480c2c1dac74cc9de7470504fbd2b80265";
  public static final String SUBMODULE_DELETE_SUBMODULE =
      "d1b1d8f07f2e99429bafda134282b97588c69b3d";
  public static final String TWO_TESTS_BRANCH =
      "two-tests-branch"; // Local only (created by the test case).
  public static final String ONE_SH_TEST = 
      "ff7e60d535564a0695a5bf9ed1774bacc480bf50"; // (v1/sh-test) add an executable shell file and BUILD.bazel file
  public static final String SH_TEST_NOT_EXECUTABLE =
      "6452291f3dcea1a5cdb332463308b70325a833e0"; // (v1/sh-test-non-executable) make shell file non-executable
  public static final String INCOMPATIBLE_TARGET =
      "69b4567d904cad46a584901c82c2959be89ae458";
}
