package com.github.bazel_contrib.target_determinator.integration;

import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;
import java.util.Set;

import org.eclipse.jgit.api.Git;
import org.eclipse.jgit.util.FileUtils;
import org.junit.After;
import org.junit.Before;
import org.junit.BeforeClass;
import org.junit.Test;

import static org.hamcrest.MatcherAssert.assertThat;
import static org.hamcrest.Matchers.containsString;

public class TargetDeterminatorOptimizationIntegrationTest {
  private static TestdataRepo testdataRepo;

  private static Path testDir;

  @BeforeClass
  public static void cloneRepo() throws Exception {
    testdataRepo = Util.cloneTestdataRepo();
    testDir = Files.createTempDirectory("target-determinator-optimization-testdata-clone");
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
  public void optimizedExecutionSkipsHashingWhenOnlyUnusedFileChanged() throws Exception {
    TestdataRepo.gitCheckout(testDir, Commits.ONE_TEST_BAZEL7_0_0);
    commitUnusedFile(testDir);

    TargetDeterminator.Result optimized =
        getResult(testDir, Commits.ONE_TEST_BAZEL7_0_0, "//java/example:ExampleTest");

    Util.assertTargetsMatch(TargetDeterminator.parseLabels(optimized.stdout()), Set.of(), Set.of(), false);
    assertThat(
        optimized.stderr(),
        containsString("Skipping hash prefill: cquery metadata is unchanged and changed files are outside the transitive source graph"));
  }

  private TargetDeterminator.Result getResult(Path workspace, String commitBefore, String targets)
      throws Exception {
    final List<String> args = new ArrayList<>(
        List.of(
            "--working-directory",
            workspace.toString(),
            "--bazel",
            "bazelisk",
            "--targets",
            targets,
            "--nocache_results",
            "--delete-cached-worktree",
            commitBefore));
    return TargetDeterminator.getResult(workspace, args.toArray(new String[0]));
  }

  private void commitUnusedFile(Path workspace) throws Exception {
    Path unusedFile = workspace.resolve("unused-file-outside-bazel-graph.txt");
    Files.writeString(unusedFile, "not referenced by any target\n");
    commitPath(workspace, unusedFile.getFileName().toString(), "Add unused file outside Bazel graph");
  }

  private void commitPath(Path workspace, String filePattern, String message) throws Exception {
    try (Git git = Git.open(workspace.toFile())) {
      git.add().addFilepattern(filePattern).call();
      git.commit()
          .setAuthor("Target Determinator Test", "target-determinator@example.invalid")
          .setCommitter("Target Determinator Test", "target-determinator@example.invalid")
          .setMessage(message)
          .call();
    }
  }
}
