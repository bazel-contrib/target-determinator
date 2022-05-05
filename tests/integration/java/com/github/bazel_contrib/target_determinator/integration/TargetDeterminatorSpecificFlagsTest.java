package com.github.bazel_contrib.target_determinator.integration;

import com.github.bazel_contrib.target_determinator.label.Label;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Set;
import org.eclipse.jgit.util.FileUtils;
import org.junit.After;
import org.junit.Before;
import org.junit.BeforeClass;
import org.junit.Test;

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
    Set<Label> targets =
        getTargets(Commits.TWO_LANGUAGES_OF_TESTS, Commits.BAZELRC_TEST_ENV, "//...");
    Util.assertTargetsMatch(
        targets,
        Set.of("//java/example:ExampleTest", "//java/example:OtherExampleTest", "//sh:sh_test"),
        Set.of(),
        false);
  }

  @Test
  public void targetPatternFlagJava() throws Exception {
    Set<Label> targets =
        getTargets(Commits.TWO_LANGUAGES_OF_TESTS, Commits.BAZELRC_TEST_ENV, "//java/...");
    Util.assertTargetsMatch(
        targets,
        Set.of("//java/example:ExampleTest", "//java/example:OtherExampleTest"),
        Set.of(),
        false);
  }

  @Test
  public void targetPatternFlagOneTarget() throws Exception {
    Set<Label> targets =
        getTargets(
            Commits.TWO_LANGUAGES_OF_TESTS, Commits.BAZELRC_TEST_ENV, "//java/example:ExampleTest");
    Util.assertTargetsMatch(targets, Set.of("//java/example:ExampleTest"), Set.of(), false);
  }

  @Test
  public void targetPatternFlagOneTargetNotAffected() throws Exception {
    Set<Label> targets =
        getTargets(
            Commits.TWO_NATIVE_TESTS_BAZEL3, Commits.TWO_TESTS, "//java/example:ExampleTest");
    Util.assertTargetsMatch(targets, Set.of("//java/example:ExampleTest"), Set.of(), false);
  }

  private Set<Label> getTargets(String commitBefore, String commitAfter, String targetPattern)
      throws Exception {
    TestdataRepo.gitCheckout(testDir, commitAfter);
    return TargetDeterminator.getTargets(
        testDir,
        "--working-directory",
        testDir.toString(),
        "--bazel",
        "bazelisk",
        "--target-pattern",
        targetPattern,
        commitBefore);
  }
}
