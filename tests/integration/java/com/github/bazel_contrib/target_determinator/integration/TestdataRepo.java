package com.github.bazel_contrib.target_determinator.integration;

import java.nio.file.Files;
import java.nio.file.Path;
import org.eclipse.jgit.api.Git;

/**
 * TestdataRepo keeps track of a clone of the testdata repo used for tests, and allows making clean
 * clones thereof for individual tests to operate in.
 */
public class TestdataRepo {
  private final Path path;

  /** Create a new TestdataRepo by cloning from upstream. */
  public static TestdataRepo create() throws Exception {
    Path path = Files.createTempDirectory("target-determinator-testdata");
    Git.cloneRepository()
        .setURI("https://github.com/illicitonion/target-determinator-testdata.git")
        .setDirectory(path.toFile())
        // We want to ensure that when using the real testdata repo, any referenced commits have matching tags.
        // We may occasionally rewrite branches' history (e.g. we did when we changed the bazel version being used by all tests), but tags are immutable.
        // There should be a tag for any commit we care about, so that we don't accidentally break test runs at historical commits in this repo.
        //
        // If you're developing a new test, you may want to set this to true (and change the URI above to a file:// URI)
        // so that the commits on the branch you're developing on get cloned into your test repo.
        // When a test has been developed and is ready to be merged into main,
        // we will push any relevant tags, and set this argument back to false.
        .setCloneAllBranches(false)
        .call();
    return new TestdataRepo(path);
  }

  /** Create a TestdataRepo against a known path of a local clone. */
  public static TestdataRepo forExistingClone(Path path) {
    return new TestdataRepo(path);
  }

  private TestdataRepo(Path path) {
    this.path = path;
  }

  /** Clones the repo to the passed path. Assumes the path is empty. */
  public void cloneTo(Path destination) throws Exception {
    Git.cloneRepository()
        .setURI(path.toString())
        .setDirectory(destination.toFile())
        .setNoCheckout(true)
        // Because in TestdataRepo.create we control whether we clone branches from upstream,
        // we set this to true here to follow whatever the create call decided.
        // If create set this to false, we won't have any branches to clone anyway so this is a no-op.
        // If create set this to true, we will clone all of the branches.
        .setCloneAllBranches(true)
        .call();
  }

  public static void gitCheckout(final Path path, final String commit) throws Exception {
    final Git gitRepository = Git.open(path.toFile());
    gitRepository.checkout().setName(commit).call();
    gitRepository.submoduleInit().call();
    gitRepository.submoduleUpdate().call();
  }

  public static void gitCheckoutBranch(final Path path, final String branch) throws Exception {
    final Git gitRepository = Git.open(path.toFile());
    gitRepository.checkout().setCreateBranch(true).setName(branch).call();
  }

  public static String gitBranch(final Path path) throws Exception {
    final Git gitRepository = Git.open(path.toFile());
    return gitRepository.getRepository().getBranch();
  }
}
