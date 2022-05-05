package com.github.bazel_contrib.target_determinator.integration;

import com.github.bazel_contrib.target_determinator.label.Label;
import com.google.common.collect.ImmutableSet;

/** TargetComputationErrorException represents an error when computing targets. */
class TargetComputationErrorException extends Exception {

  private final ImmutableSet<Label> targets;

  /** getTargets returns any targets which were output to stdout. */
  public ImmutableSet<Label> getTargets() {
    return targets;
  }

  public TargetComputationErrorException(String errorMessage, ImmutableSet<Label> targets) {
    super(errorMessage);
    this.targets = targets;
  }
}
