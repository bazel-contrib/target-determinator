package com.github.bazel_contrib.target_determinator.integration;

import com.github.bazel_contrib.target_determinator.label.Label;
import com.google.common.base.Joiner;
import com.google.common.collect.ImmutableSet;
import com.google.common.collect.ImmutableSet.Builder;
import com.google.common.io.ByteStreams;
import java.io.File;
import java.io.IOException;
import java.lang.ProcessBuilder.Redirect;
import java.nio.charset.StandardCharsets;
import java.nio.file.Path;
import java.util.Set;

/** Wrapper around a target-determinator binary. */
public class TargetDeterminator {
  private static final String TARGET_DETERMINATOR =
      new File(System.getProperty("target_determinator")).getAbsolutePath();

  /** Get the targets returned by a run of target-determinator. */
  public static Set<Label> getTargets(Path workspace, String... argv)
      throws TargetComputationErrorException {
    return runProcess(workspace, TARGET_DETERMINATOR, argv);
  }

  private static ImmutableSet<Label> runProcess(Path workingDirectory, String argv0, String... argv)
      throws TargetComputationErrorException {
    ProcessBuilder processBuilder = new ProcessBuilder(argv0);
    for (String arg : argv) {
      processBuilder.command().add(arg);
    }
    processBuilder.directory(workingDirectory.toFile());
    processBuilder.redirectOutput(Redirect.PIPE);
    processBuilder.redirectError(Redirect.INHERIT);
    // Do not clean the environment, so we can inherit variables passed e.g. via --test_env.
    // Useful for CC (needed by bazel).
    processBuilder.environment().put("HOME", System.getProperty("user.home"));
    processBuilder.environment().put("PATH", System.getenv("PATH"));
    try {
      Process process = processBuilder.start();
      Builder<Label> builder = new Builder<>();
      for (String line :
          new String(ByteStreams.toByteArray(process.getInputStream()), StandardCharsets.UTF_8)
              .split("\n")) {
        if (!line.isEmpty()) {
          builder.add(Label.normalize(line));
        }
      }
      final ImmutableSet<Label> targets = builder.build();
      final int returnCode = process.waitFor();
      if (returnCode != 0) {
        throw new TargetComputationErrorException(
            String.format(
                "Expected exit code 0 when running %s but got: %d",
                Joiner.on(" ").join(argv), returnCode),
            targets);
      }
      return targets;
    } catch (IOException | InterruptedException e) {
      throw new RuntimeException(e);
    }
  }
}
