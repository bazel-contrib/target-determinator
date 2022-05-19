package com.github.bazel_contrib.target_determinator.integration;

import com.github.bazel_contrib.target_determinator.label.Label;
import java.nio.file.Files;
import java.nio.file.Path;
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
            Commits.TWO_NATIVE_TESTS_BAZEL3, "//java/example:ExampleTest");
    Util.assertTargetsMatch(targets, Set.of("//java/example:ExampleTest"), Set.of(), false);
  }

  @Test
  public void failForUncleanRepositoryWithEnforceClean() throws Exception {
    TestdataRepo.gitCheckout(testDir, Commits.HAS_JVM_FLAGS);

    Files.createFile(testDir.resolve("untracked-file"));

    try {
      getTargets(Commits.TWO_TESTS, "//...", true);
      fail("Expected target-determinator command to fail but it succeeded");
    } catch (TargetComputationErrorException e) {
      assertThat(e.getOutput(), CoreMatchers.equalTo("Target Determinator invocation Error\n"));
    }
  }

  @Test
  public void succeedForUncleanIgnoredFilesWithEnforceClean() throws Exception {
    TestdataRepo.gitCheckout(testDir, Commits.TWO_TESTS_WITH_GITIGNORE);

    Path ignoredFile = testDir.resolve("ignored-file");
    Files.createFile(ignoredFile);

    Set<Label> targets = getTargets(Commits.ONE_TEST, "//...", true);
    Util.assertTargetsMatch(targets, Set.of("//java/example:OtherExampleTest"), Set.of(), false);

    assertThat("expected ignored file to still be present after invocation", ignoredFile.toFile().exists());
  }

  @Test
  public void failForUncleanSubmoduleWithEnforceClean() throws Exception {
    TestdataRepo.gitCheckout(testDir, Commits.SUBMODULE_CHANGE_DIRECTORY);

    Files.createFile(testDir.resolve("demo-submodule-2").resolve("untracked-file"));

    try {
      getTargets(Commits.SUBMODULE_ADD_DEPENDENT_ON_SIMPLE_JAVA_LIBRARY,
          "//...", true);
      fail("Expected target-determinator command to fail but it succeeded");
    } catch (TargetComputationErrorException e) {
      assertThat(e.getOutput(), CoreMatchers.equalTo("Target Determinator invocation Error\n"));
    }
  }

  private Set<Label> getTargets(String commitBefore, String targetPattern) throws Exception {
    return getTargets(commitBefore, targetPattern, false);
  }

  private Set<Label> getTargets(String commitBefore, String targetPattern, boolean enforceClean)
      throws Exception {
    final List<String> args = Stream.of("--working-directory", testDir.toString(), "--bazel", "bazelisk",
            "--target-pattern", targetPattern).collect(Collectors.toList());
    if (enforceClean) {
      args.add("--enforce-clean=enforce-clean");
    }
    args.add(commitBefore);
    return TargetDeterminator.getTargets(testDir, args.toArray(new String[0]));
  }
}
