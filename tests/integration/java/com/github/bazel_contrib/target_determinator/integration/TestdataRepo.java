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
        .setURI("https://github.com/bazel-contrib/target-determinator-testdata.git")
        .setDirectory(path.toFile())
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
