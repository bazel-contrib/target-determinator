package com.github.bazel_contrib.target_determinator.integration;

/** TargetComputationErrorException represents an error when computing targets. */
class TargetComputationErrorException extends Exception {

  private final String output;

  /**
   * getOutput returns the stdout of the failed command.
   */
  public String getOutput() {
    return output;
  }

  public TargetComputationErrorException(String errorMessage, String output) {
    super(errorMessage);
    this.output = output;
  }
}
