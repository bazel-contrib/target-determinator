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
    // Between Bazel 3 and 5, the attributes available on java_test changed, which means
    // these targets may be picked up as changed if a target determinator is just doing
    // hashing of target-local data.
    // However, the PatchVersion tests are also significant - changing Bazel versions may change
    // how RuleClasses are interpreted, or internal implementation details of them not captured
    // by their query-observable interface.
    doTest(
        Commits.TWO_TESTS,
        Commits.TWO_NATIVE_TESTS_BAZEL3,
        Set.of("//java/example:ExampleTest", "//java/example:OtherExampleTest"));
  }

  @Test
  public void changedBazelPatchVersion_native() throws Exception {
    doTest(
        Commits.TWO_TESTS,
        Commits.TWO_NATIVE_TESTS_BAZEL5_1_0,
        Set.of("//java/example:ExampleTest", "//java/example:OtherExampleTest"));
  }

  @Test
  public void changedBazelMajorVersion_starlark() throws Exception {
    doTest(
        Commits.SIMPLE_JAVA_LIBRARY_TARGETS,
        Commits.SIMPLE_TARGETS_BAZEL3,
        Set.of("//java/example/simple:simple", "//java/example/simple:simple_dep"));
  }

  @Test
  public void changedBazelPatchVersion_starlark() throws Exception {
    doTest(
        Commits.SIMPLE_JAVA_LIBRARY_TARGETS,
        Commits.SIMPLE_TARGETS_BAZEL5_1_0,
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
  public void tryImportInBazelrcAffectingJava() throws Exception {
    doTest(
        Commits.TWO_LANGUAGES_OF_TESTS,
        Commits.TWO_LANGUAGES_OPTIONAL_MISSING_TRY_IMPORT,
        Set.of());
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
    try {
      doTest(
          Commits.ADD_INDIRECTION_FOR_SIMPLE_JAVA_LIBRARY,
          Commits.REDUCE_DEPENDENCY_VISIBILITY,
          Set.of("//NotApplicable"));
      fail("Expected target-determinator command to fail but it succeeded");
    } catch (TargetComputationErrorException e) {
      // Invocation failed as expected.
    }
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
  public void testChmodFile() throws TargetComputationErrorException {
    doTest(Commits.ONE_SH_TEST, Commits.SH_TEST_NOT_EXECUTABLE, Set.of("//sh:sh_test"));
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
  public static final String ONE_TEST = "21024914188b0a8aaf88f81a5b9dfbdf3b24dca5";
  public static final String TWO_TESTS = "d00fdc57fad09fbdc1a9b9e53ce0a102e813fd1a";
  public static final String HAS_JVM_FLAGS = "3d22ee76c892762fc979eaf0be10019f56c82995";
  public static final String EXPLICIT_DEFAULT_VALUE = "825ec627626fc910ed21bf62241fa96e9aa0c54c";
  public static final String TWO_NATIVE_TESTS_BAZEL3 = "9bb3a36e3e139b9f125d64d35e6da7e712e5f606";
  public static final String TWO_NATIVE_TESTS_BAZEL5_1_0 = "a9d746c5ff071658a461005577dfd3052d2b212d";
  public static final String MODIFIED_TEST_SRC = "36b10bfc8e4cac62e3471115ab49d0a981b736f6";
  public static final String TWO_LANGUAGES_OF_TESTS = "b93e37329f1e2fc01b99bfcadc5816be8db25b44";
  public static final String BAZELRC_TEST_ENV = "a3d71cfcf64ae1eb6b6ef55268b47fa5cf41b6ff";
  public static final String BAZELRC_AFFECTING_JAVA = "e84173a8937f141c174bc195d77ac5cf845035f1";
  public static final String SIMPLE_TARGETS_BAZEL3 = "69276dbac636812501212237871a3f8fdbd71519";
  public static final String SIMPLE_TARGETS_BAZEL5_1_0 = "d7dd0a66dbfa9857b2bb642a9cc1ae8103ce50b0";
  public static final String ADD_OPTIONAL_PRESENT_EMPTY_BAZELRC =
      "32e4a4533d752781d76d36d0ec65d74558aa5574";
  public static final String SIMPLE_JAVA_LIBRARY_RULE = "87236b8d878ef596bcb3938c85a850d031ac7fec";
  public static final String SIMPLE_JAVA_LIBRARY_TARGETS =
      "053b1302b554df6fafe5c5fa3c812b625a58c08f";
  public static final String SIMPLE_JAVA_LIBRARY_AND_JAVA_TESTS =
      "991771fd338e57796065445a89782aaaee79c811";
  public static final String CHANGE_TRANSITIVE_FILE = "3e9977e9d3b9c6e181053b35c332b77d54172e39";
  public static final String TWO_LANGUAGES_OPTIONAL_MISSING_TRY_IMPORT =
      "6d1773a0bb6cdbaa0c13273d76b3e3a474198e19";
  public static final String TWO_LANGUAGES_OPTIONAL_PRESENT_BAZELRC_AFFECTING_JAVA =
      "4e132ae57ab18aaa7df56e15a574248caa2d9419";
  public static final String TWO_LANGUAGES_NOOP_IMPORTED_BAZELRC =
      "7f5cbe1855d785ff08006f1800e534aa4543130e";
  public static final String TWO_LANGUAGES_IMPORTED_BAZELRC_AFFECTING_JAVA =
      "16e1936b701180aa5f55caaa1afef42a9d3332db";
  public static final String JAVA_TESTS_AND_SIMPLE_JAVA_RULES =
      "319e09542480559f3a7fbdba0abdc5399e4d5d2f";
  public static final String DEP_ON_STARLARK_TARGET = "427cf9f5ece1ac7c358d8dfaeb920e94070bfd71";
  public static final String CHANGE_STARLARK_RULE_IMPLEMENTATION =
      "cc7d8a8842712334fbac1e57b9f6639d84182e3f";
  public static final String NOOP_REFACTOR_STARLARK_RULE_IMPLEMENTATION =
      "edaa8a768f69f2fea89affc65564a6ff486b0700";
  public static final String RULES_IN_EXTERNAL_REPO = "1054f4bc492268addbcf4043ea32965eae76304e";
  public static final String NOOP_REFACTOR_IN_WORKSPACE_FILE =
      "e9d79d954586316f240e0f21d4b364d03ac53ec6";
  public static final String ADD_SIMPLE_PACKAGE_RULE = "1e6f63045322ec64785d1044c77a21d7297ec90c";
  public static final String REFACTORED_WORKSPACE_INDIRECTLY =
      "e7d1ecdf82b1ef248201906cece48ccd81870dd2";
  public static final String PATHOLOGICAL_RULES_SINGLE_TARGET =
      "47664af1266b0f4c95b97ae5e6c7d0215d27abd6";
  public static final String PATHOLOGICAL_RULES_TWO_TARGETS =
      "4a51052d5e37953441216f253de3c7e45b814b35";
  public static final String PATHOLOGICAL_RULES_THREE_TARGETS =
      "d054b4b5461ae79864a5767db582049de261f45c";
  public static final String PATHOLOGICAL_RULES_FIVE_TARGETS =
      "e22ac7985b3651203653dcfb3123d7e90276a7ad";
  public static final String CHANGE_ATTRIBUTES_VIA_INDIRECTION =
      "89f8ba981fc6bdd98f3a283686f4f0907e9e0ab8";
  public static final String HAS_GLOBS = "6d7345ee77529ec50832a268b1e1382d6dac2846";
  public static final String CHANGE_GLOBS = "84a87db76ce4e082ad5095881d9bf7230d43e193";
  public static final String ADD_BUILD_FILE_INTERFERING_WTH_GLOBS =
      "2f68f0e761963b1eae163be3270b55e6aa3cac1b";
  public static final String BAZELRC_INCLUDED_EMPTY = "288a6f76b28d4a37c598e74f5c29491d5da56f49";
  public static final String JAVA_USED_IN_GENRULE = "e9a4432e49ba9d8ceac7a496aa70f2646d358ab2";
  public static final String BAZELRC_INCLUDED_JAVACOPT = "a2f9cb9a7d20dd69585fe2a262c73f7fd6442ed8";
  public static final String BAZELRC_HOST_JAVACOPT = "f6b6eacd29f04b06eabd189c312dcfcc227519b1";
  public static final String ADD_INDIRECTION_FOR_SIMPLE_JAVA_LIBRARY =
      "42e7ffb4d37ba3a80684115bffcb44e6d1639d64";
  public static final String REDUCE_DEPENDENCY_VISIBILITY =
      "72228ac1191dc4b3cbc357e3bf0abce8a55450ed";
  public static final String TWO_TESTS_WITH_GITIGNORE = "698cfb887aa6318bf22d5d27914cb917ecda4499";
  public static final String SUBMODULE_ADD_TRIVIAL_SUBMODULE =
      "44ea0dd38b06cbac069a5799f7b7d560b420b13f";
  public static final String SUBMODULE_ADD_DEPENDENT_ON_SIMPLE_JAVA_LIBRARY =
      "7afffd90c703f6e0ac3cb6a853bdf94d5ba39f43";
  public static final String SUBMODULE_CHANGE_DIRECTORY =
      "c8b244641693ddd180ab12d183f5be21dfcfd8c6";
  public static final String SUBMODULE_DELETE_SUBMODULE =
      "dde94a13e0f6f9a970bcaf700c45fc4ecb4e7949";
  public static final String TWO_TESTS_BRANCH =
      "two-tests-branch"; // Local only (created by the test case).
  public static final String ONE_SH_TEST = 
      "106ac1dd1fd762c3786e1f01b2aa47fc1eccab99"; // (v0/sh-test) add an executable shell file and BUILD.bazel file
  public static final String SH_TEST_NOT_EXECUTABLE = 
      "845171a918115260b50fb487e93c82d49e62abf6"; // (v0/sh-test-non-executable) make shell file non-executable
}
