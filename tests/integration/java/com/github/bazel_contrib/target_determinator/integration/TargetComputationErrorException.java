package com.github.bazel_contrib.target_determinator.integration;

/** TargetComputationErrorException represents an error when computing targets. */
class TargetComputationErrorException extends Exception {

  private final String stdout;
  private final String stderr;

  /**
   * getStdout returns the stdout of the failed command.
   */
  public String getStdout() {
    return stdout;
  }

  /**
   * getStderr returns the stdout of the failed command.
   */
  public String getStderr() {
    return stderr;
  }

  public TargetComputationErrorException(String errorMessage, String stdout, String stderr) {
    super(errorMessage);
    this.stdout = stdout;
    this.stderr = stderr;
  }

  @Override
  public String getMessage() {
    return String.format("%s, stderr: %s", super.getMessage(), stderr);
  }
}
