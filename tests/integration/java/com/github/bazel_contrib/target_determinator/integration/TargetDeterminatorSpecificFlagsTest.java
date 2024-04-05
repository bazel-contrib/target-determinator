package com.github.bazel_contrib.target_determinator.integration;

import com.github.bazel_contrib.target_determinator.label.Label;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;
import java.util.Set;
import java.util.stream.Collectors;
import java.util.stream.Stream;

import org.eclipse.jgit.util.FileUtils;
import org.hamcrest.CoreMatchers;
import org.junit.After;
import org.junit.Before;
import org.junit.BeforeClass;
import org.junit.Test;

import static junit.framework.TestCase.fail;
import static org.hamcrest.MatcherAssert.assertThat;
import static org.hamcrest.Matchers.containsString;
import static org.hamcrest.Matchers.equalTo;

public class TargetDeterminatorSpecificFlagsTest {
  private static TestdataRepo testdataRepo;

  // Contains a new clone of the testdata repository each time a test is run.
  private static Path testDir;

  @BeforeClass
  public static void cloneRepo() throws Exception {
    testdataRepo = Util.cloneTestdataRepo();
    testDir = Files.createTempDirectory("target-determinator-testdata_dir-clone");
  }

  @Before
  public void createTestRepository() throws Exception {
    testdataRepo.cloneTo(testDir);
  }

  @After
  public void cleanupTestRepository() throws Exception {
    FileUtils.delete(testDir.toFile(), FileUtils.RECURSIVE | FileUtils.SKIP_MISSING);
  }

  @Test
  public void targetPatternFlagAll() throws Exception {
    TestdataRepo.gitCheckout(testDir, Commits.BAZELRC_TEST_ENV);
    Set<Label> targets =
        getTargets(Commits.TWO_LANGUAGES_OF_TESTS, "//...");
    Util.assertTargetsMatch(
        targets,
        Set.of("//java/example:ExampleTest", "//java/example:OtherExampleTest", "//sh:sh_test"),
        Set.of(),
        false);
  }

  @Test
  public void targetPatternFlagJava() throws Exception {
    TestdataRepo.gitCheckout(testDir, Commits.BAZELRC_TEST_ENV);
    Set<Label> targets = getTargets(Commits.TWO_LANGUAGES_OF_TESTS, "//java/...");
    Util.assertTargetsMatch(
        targets,
        Set.of("//java/example:ExampleTest", "//java/example:OtherExampleTest"),
        Set.of(),
        false);
  }

  @Test
  public void targetPatternFlagOneTarget() throws Exception {
    TestdataRepo.gitCheckout(testDir, Commits.BAZELRC_TEST_ENV);
    Set<Label> targets = getTargets(Commits.TWO_LANGUAGES_OF_TESTS, "//java/example:ExampleTest");
    Util.assertTargetsMatch(targets, Set.of("//java/example:ExampleTest"), Set.of(), false);
  }

  @Test
  public void targetPatternFlagOneTargetNotAffected() throws Exception {
    TestdataRepo.gitCheckout(testDir, Commits.TWO_TESTS);
    Set<Label> targets =
        getTargets(
            Commits.TWO_NATIVE_TESTS_BAZEL5_4_0, "//java/example:ExampleTest");
    Util.assertTargetsMatch(targets, Set.of("//java/example:ExampleTest"), Set.of(), false);
  }

  @Test
  public void targetPatternFlagQueryBeforeWasError() throws Exception {
    TestdataRepo.gitCheckout(testDir, Commits.ONE_TEST);
    Set<Label> targets = getTargets(Commits.NO_TARGETS, "//java/...");
    Util.assertTargetsMatch(targets, Set.of("//java/example:ExampleTest"), Set.of(), false);
  }

  @Test
  public void targetPatternFlagQueryBeforeWasErrorVerbose() throws Exception {
    TestdataRepo.gitCheckout(testDir, Commits.ONE_TEST);
    String output = getOutput(Commits.NO_TARGETS, "//java/...", false, true, List.of("--verbose"));
    // This isn't great output, and we shouldn't worry about changing its format in the future,
    // but this test is to ensure we return a result indicating "the query before was bad" rather
    // than "this target didn't exist before".
    assertThat(output, equalTo("//java/example:ExampleTest Changes: ErrorInQueryBefore\n"));
  }

  @Test
  public void targetPatternFlagQueryBeforeWasErrorWhenFatal() throws Exception {
    TestdataRepo.gitCheckout(testDir, Commits.ONE_TEST);
    try {
      String output = getOutput(Commits.NO_TARGETS, "//java/...", false, true, List.of("--before-query-error-behavior=fatal"));
      fail(String.format("Expected exception but got successful output: %s", output));
    } catch (TargetComputationErrorException e) {
      assertThat(e.getStdout(), CoreMatchers.equalTo("Target Determinator invocation Error\n"));
      assertThat(e.getStderr(), containsString("failed to query at revision 'before'"));
    }
  }

  @Test
  public void failForUncleanRepositoryWithEnforceClean() throws Exception {
    TestdataRepo.gitCheckout(testDir, Commits.HAS_JVM_FLAGS);

    Files.createFile(testDir.resolve("untracked-file"));

    try {
      getTargets(Commits.TWO_TESTS, "//...", true, true);
      fail("Expected target-determinator command to fail but it succeeded");
    } catch (TargetComputationErrorException e) {
      assertThat(e.getStdout(), CoreMatchers.equalTo("Target Determinator invocation Error\n"));
    }
  }

  @Test
  public void ignoresIgnoredFile() throws Exception {
    TestdataRepo.gitCheckout(testDir, Commits.TWO_TESTS_WITH_GITIGNORE);

    Path ignoredFile = testDir.resolve("ignored-file");
    Files.createFile(ignoredFile);

    Set<Label> targets = getTargets(Commits.ONE_TEST_WITH_GITIGNORE, "//...", true, true);
    Util.assertTargetsMatch(targets, Set.of("//java/example:OtherExampleTest"), Set.of(), false);

    assertThat("expected ignored file to still be present after invocation", ignoredFile.toFile().exists());
  }

  @Test
  public void failsIfChangingCommitsCausesAnIgnoredFileToBecomeUntracked() throws Exception {
    TestdataRepo.gitCheckout(testDir, Commits.TWO_TESTS_WITH_GITIGNORE);

    Path ignoredFile = testDir.resolve("ignored-file");
    Files.createFile(ignoredFile);

    try {
      getTargets(Commits.ONE_TEST, "//...", true, true);
      fail("Expected target-determinator command to fail but it succeeded");
    } catch (TargetComputationErrorException e) {
      assertThat(e.getStdout(), CoreMatchers.equalTo("Target Determinator invocation Error\n"));
      assertThat(e.getStderr(), containsString("repository was not clean after checking out revision 'before'"));
    }
  }

  @Test
  public void failForUncleanSubmoduleWithEnforceClean() throws Exception {
    TestdataRepo.gitCheckout(testDir, Commits.SUBMODULE_CHANGE_DIRECTORY);

    Files.createFile(testDir.resolve("demo-submodule-2").resolve("untracked-file"));

    try {
      getTargets(Commits.SUBMODULE_ADD_DEPENDENT_ON_SIMPLE_JAVA_LIBRARY,
          "//...", true, true);
      fail("Expected target-determinator command to fail but it succeeded");
    } catch (TargetComputationErrorException e) {
      assertThat(e.getStdout(), CoreMatchers.equalTo("Target Determinator invocation Error\n"));
    }
  }

  @Test
  public void testWorktreeCreation() throws Exception {
    TestdataRepo.gitCheckout(testDir, Commits.TWO_TESTS);
    // Make repository unclean so that a worktree gets created.
    Files.createFile(testDir.resolve("untracked-file"));
    getTargets(Commits.SUBMODULE_ADD_DEPENDENT_ON_SIMPLE_JAVA_LIBRARY,
        "//...", false, false);

    Path worktreePath = TargetDeterminator.getWorktreePath(testDir);
    assertThat("Expected cached git worktree to be present", Files.exists(worktreePath.resolve(".git")));

    getTargets(Commits.SUBMODULE_ADD_DEPENDENT_ON_SIMPLE_JAVA_LIBRARY,
        "//...", false, true);
    assertThat("Expected cached git worktree to be absent", !Files.exists(worktreePath.resolve(".git")));

  }

  @Test
  public void changedConfigurationVerbose() throws Exception {
    TestdataRepo.gitCheckout(testDir, Commits.BAZELRC_AFFECTING_JAVA);
    String output = getOutput(Commits.TWO_LANGUAGES_OF_TESTS, "//java/example:ExampleTest", false, true, List.of("--verbose"));
    // This isn't great output, and we shouldn't worry about changing its format in the future,
    // but this test is to ensure we return a result including a hint as to what changed the
    // configuration.
    assertThat(output, containsString("-source 7 -target 7"));
  }

  @Test
  public void startupOptsIgnoringBazelrc() throws Exception {
    TestdataRepo.gitCheckout(testDir, Commits.BAZELRC_TEST_ENV);
    Set<Label> targets = getTargets(Commits.TWO_LANGUAGES_OF_TESTS, "//...", false, true, List.of("--bazel-startup-opts=--noworkspace_rc"));
    Util.assertTargetsMatch(targets, Set.of(), Set.of(), false);
  }

  private Set<Label> getTargets(String commitBefore, String targets) throws Exception {
    return getTargets(commitBefore, targets, false, true);
  }

  private Set<Label> getTargets(String commitBefore, String targets, boolean enforceClean, boolean deleteCachedWorktree)
      throws Exception {
    return getTargets(commitBefore, targets, enforceClean, deleteCachedWorktree, new ArrayList<>());
  }

  private Set<Label> getTargets(String commitBefore, String targets, boolean enforceClean, boolean deleteCachedWorktree, List<String> flags)
      throws Exception {
    return TargetDeterminator.parseLabels(getOutput(commitBefore, targets, enforceClean, deleteCachedWorktree, flags));
  }

  private String getOutput(String commitBefore, String targets, boolean enforceClean, boolean deleteCachedWorktree, List<String> flags) throws Exception {
    final List<String> args = Stream.concat(
        Stream.of("--working-directory",
            testDir.toString(),
            "--bazel", "bazelisk",
            "--targets", targets
        ),
        flags.stream()
    ).collect(Collectors.toList());
    if (enforceClean) {
      args.add("--enforce-clean=enforce-clean");
    }
    if (deleteCachedWorktree) {
      args.add("--delete-cached-worktree");
    }
    args.add(commitBefore);
    return TargetDeterminator.getOutput(testDir, args.toArray(new String[0]));
  }
}
