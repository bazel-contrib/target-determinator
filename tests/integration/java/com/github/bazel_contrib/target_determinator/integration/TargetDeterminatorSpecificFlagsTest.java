package com.github.bazel_contrib.target_determinator.integration;

import com.github.bazel_contrib.target_determinator.label.Label;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;
import java.util.Set;
import java.util.stream.Collectors;
import java.util.stream.Stream;

import org.junit.Before;
import org.junit.Rule;
import org.junit.Test;
import org.junit.rules.TemporaryFolder;

import static junit.framework.TestCase.fail;
import static org.hamcrest.MatcherAssert.assertThat;
import static org.hamcrest.Matchers.containsString;
import static org.hamcrest.Matchers.equalTo;

public class TargetDeterminatorSpecificFlagsTest {

  @Rule
  public TemporaryFolder rootFolder = new TemporaryFolder();
  private TestRepo repo;

  @Before
  public void createTestRepository() throws IOException {
    Path dir = rootFolder.newFolder().toPath();
    repo = TestRepo.create(dir);
  }

  @Test
  public void targetPatternFlagAll() throws Exception {
    repo.replaceWithContentsFrom(Commits.TWO_LANGUAGES_OF_TESTS);
    String beforeCommit = repo.commit("Before commit");

    repo.replaceWithContentsFrom(Commits.BAZELRC_TEST_ENV);
    repo.commit("After commit");

    Set<Label> targets = getTargets(beforeCommit, "//...");
    Util.assertTargetsMatch(
        targets,
        Set.of("//java/example:ExampleTest", "//java/example:OtherExampleTest", "//sh:sh_test"),
        Set.of(),
        false);
  }

  @Test
  public void targetPatternFlagJava() throws Exception {
    repo.replaceWithContentsFrom(Commits.TWO_LANGUAGES_OF_TESTS);
    String beforeCommit = repo.commit("Before commit");

    repo.replaceWithContentsFrom(Commits.BAZELRC_TEST_ENV);
    repo.commit("After commit");

    Set<Label> targets = getTargets(beforeCommit, "//java/...");
    Util.assertTargetsMatch(
        targets,
        Set.of("//java/example:ExampleTest", "//java/example:OtherExampleTest"),
        Set.of(),
        false);
  }

  @Test
  public void targetPatternFlagOneTarget() throws Exception {
    repo.replaceWithContentsFrom(Commits.TWO_LANGUAGES_OF_TESTS);
    String beforeCommit = repo.commit("Before commit");

    repo.replaceWithContentsFrom(Commits.BAZELRC_TEST_ENV);
    repo.commit("After commit");

    Set<Label> targets = getTargets(beforeCommit, "//java/example:ExampleTest");
    Util.assertTargetsMatch(targets, Set.of("//java/example:ExampleTest"), Set.of(), false);
  }

  @Test
  public void targetPatternFlagOneTargetNotAffected() throws Exception {
    repo.replaceWithContentsFrom(Commits.TWO_NATIVE_TESTS_BAZEL5_4_0);
    String beforeCommit = repo.commit("Before commit");

    repo.replaceWithContentsFrom(Commits.TWO_TESTS);
    repo.commit("After commit");

    Set<Label> targets = getTargets(beforeCommit, "//java/example:ExampleTest");
    Util.assertTargetsMatch(targets, Set.of("//java/example:ExampleTest"), Set.of(), false);
  }

  @Test
  public void targetPatternFlagQueryBeforeWasError() throws Exception {
    repo.replaceWithContentsFrom(Commits.NO_TARGETS);
    String beforeCommit = repo.commit("Before commit");

    repo.replaceWithContentsFrom(Commits.ONE_TEST);
    repo.commit("After commit");

    Set<Label> targets = getTargets(beforeCommit, "//java/...");
    Util.assertTargetsMatch(targets, Set.of("//java/example:ExampleTest"), Set.of(), false);
  }

  @Test
  public void targetPatternFlagQueryBeforeWasErrorVerbose() throws Exception {
    repo.replaceWithContentsFrom(Commits.NO_TARGETS);
    String beforeCommit = repo.commit("Before commit");

    repo.replaceWithContentsFrom(Commits.ONE_TEST);
    repo.commit("After commit");

    String output = getOutput(beforeCommit, "//java/...", false, true, List.of("--verbose"));
    // This isn't great output, and we shouldn't worry about changing its format in the future,
    // but this test is to ensure we return a result indicating "the query before was bad" rather
    // than "this target didn't exist before".
    assertThat(output, equalTo("//java/example:ExampleTest Changes: ErrorInQueryBefore\n"));
  }

  @Test
  public void targetPatternFlagQueryBeforeWasErrorWhenFatal() throws Exception {
    repo.replaceWithContentsFrom(Commits.NO_TARGETS);
    String beforeCommit = repo.commit("Before commit");

    repo.replaceWithContentsFrom(Commits.ONE_TEST);
    repo.commit("After commit");
    try {
      String output = getOutput(beforeCommit, "//java/...", false, true, List.of("--before-query-error-behavior=fatal"));
      fail(String.format("Expected exception but got successful output: %s", output));
    } catch (TargetComputationErrorException e) {
      assertThat(e.getStdout(), equalTo("Target Determinator invocation Error\n"));
      assertThat(e.getStderr(), containsString("failed to query at revision 'before'"));
    }
  }

  @Test
  public void failForUncleanRepositoryWithEnforceClean() throws Exception {
    repo.replaceWithContentsFrom(Commits.TWO_TESTS);
    String beforeCommit = repo.commit("Before commit");

    repo.replaceWithContentsFrom(Commits.HAS_JVM_FLAGS);
    repo.commit("After commit");

    Files.createFile(repo.getDir().resolve("untracked-file"));

    try {
      getTargets(beforeCommit, "//...", true, true);
      fail("Expected target-determinator command to fail but it succeeded");
    } catch (TargetComputationErrorException e) {
      assertThat(e.getStdout(), equalTo("Target Determinator invocation Error\n"));
    }
  }

  @Test
  public void ignoresIgnoredFile() throws Exception {
    repo.replaceWithContentsFrom(Commits.ONE_TEST_WITH_GITIGNORE);
    String beforeCommit = repo.commit("Before commit");

    repo.replaceWithContentsFrom(Commits.TWO_TESTS_WITH_GITIGNORE);
    repo.commit("After commit");

    Path ignoredFile = repo.getDir().resolve("ignored-file");
    Files.createFile(ignoredFile);

    Set<Label> targets = getTargets(beforeCommit, "//...", true, true);
    Util.assertTargetsMatch(targets, Set.of("//java/example:OtherExampleTest"), Set.of(), false);

    assertThat("expected ignored file to still be present after invocation", ignoredFile.toFile().exists());
  }

  @Test
  public void failsIfChangingCommitsCausesAnIgnoredFileToBecomeUntracked() throws Exception {
    repo.replaceWithContentsFrom(Commits.ONE_TEST);
    String beforeCommit = repo.commit("Before commit");

    repo.replaceWithContentsFrom(Commits.TWO_TESTS_WITH_GITIGNORE);
    repo.commit("After commit");

    Path ignoredFile = repo.getDir().resolve("ignored-file");
    Files.createFile(ignoredFile);

    try {
      getTargets(beforeCommit, "//...", true, true);
      fail("Expected target-determinator command to fail but it succeeded");
    } catch (TargetComputationErrorException e) {
      assertThat(e.getStdout(), equalTo("Target Determinator invocation Error\n"));
      assertThat(e.getStderr(), containsString("repository was not clean after checking out revision 'before'"));
    }
  }

  @Test
  public void failForUncleanSubmoduleWithEnforceClean() throws Exception {
    Path submodulePath = rootFolder.newFolder().toPath();
    TestRepo submodule = TestRepo.create(submodulePath);
    submodule.replaceWithContentsFrom(Commits.EMPTY_SUBMODULE);
    submodule.commit("Initial commit");

    repo.replaceWithContentsFrom(Commits.SIMPLE_JAVA_LIBRARY_TARGETS);
    repo.commit("Before commit");

    TestRepo submoduleWithinRepo = repo.addSubModule(submodule, "demo-submodule");
    repo.commit("Add a submodule");

    submodule.replaceWithContentsFrom(Commits.ADD_DEPENDENT_ON_SIMPLE_JAVA_LIBRARY);
    submodule.commit("Add dependent of simple java library");

    submoduleWithinRepo.pull();

    String beforeCommit = repo.commit("Add dependent target in submodule", "demo-submodule");

    repo.moveSubmodule("demo-submodule", "demo-submodule-2");
    repo.commit("Move demo-submodule to demo-submodule-2");

    Files.createFile(repo.getDir().resolve("demo-submodule-2").resolve("untracked-file"));

    try {
      getTargets(beforeCommit, "//...", true, true);
      fail("Expected target-determinator command to fail but it succeeded");
    } catch (TargetComputationErrorException e) {
      assertThat(e.getStdout(), equalTo("Target Determinator invocation Error\n"));
    }
  }

  @Test
  public void testWorktreeCreation() throws Exception {
    repo.replaceWithContentsFrom(Commits.ONE_TEST);
    String beforeCommit = repo.commit("Before commit");

    repo.replaceWithContentsFrom(Commits.TWO_TESTS);
    repo.commit("After commit");

    // Make repository unclean so that a worktree gets created
    Files.createFile(repo.getDir().resolve("untracked-file"));

    getTargets(beforeCommit, "//...", false, false);

    Path worktreePath = TargetDeterminator.getWorktreePath(repo.getDir());
    assertThat("Expected cached git worktree to be present", Files.exists(worktreePath.resolve(".git")));

    getTargets(beforeCommit, "//...", false, true);
    assertThat("Expected cached git worktree to be absent", !Files.exists(worktreePath.resolve(".git")));
  }

  @Test
  public void changedConfigurationVerbose() throws Exception {
    repo.replaceWithContentsFrom(Commits.TWO_LANGUAGES_OF_TESTS);
    String beforeCommit = repo.commit("Before commit");

    repo.replaceWithContentsFrom(Commits.BAZELRC_AFFECTING_JAVA);
    repo.commit("After commit");

    String output = getOutput(beforeCommit, "//java/example:ExampleTest", false, true, List.of("--verbose"));
    // This isn't great output, and we shouldn't worry about changing its format in the future,
    // but this test is to ensure we return a result including a hint as to what changed the
    // configuration.
    assertThat(output, containsString("-source 7 -target 7"));
  }

  @Test
  public void startupOptsIgnoringBazelrc() throws Exception {
    repo.replaceWithContentsFrom(Commits.TWO_LANGUAGES_OF_TESTS);
    String beforeCommit = repo.commit("Before commit");

    repo.replaceWithContentsFrom(Commits.BAZELRC_TEST_ENV);
    repo.commit("After commit");

    Set<Label> targets = getTargets(beforeCommit, "//...", false, true, List.of("--bazel-startup-opts=--noworkspace_rc"));
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
            repo.getDir().toString(),
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
    return TargetDeterminator.getOutput(repo.getDir(), args.toArray(new String[0]));
  }
}
