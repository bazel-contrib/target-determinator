package com.github.bazel_contrib.target_determinator.integration;

import static org.hamcrest.MatcherAssert.assertThat;
import static org.hamcrest.Matchers.not;
import static org.hamcrest.collection.IsEmptyCollection.empty;

import com.github.bazel_contrib.target_determinator.label.Label;
import com.google.common.collect.ImmutableSet;
import com.google.common.collect.ImmutableSet.Builder;
import com.google.common.collect.Sets;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.Set;
import java.util.stream.Collectors;

public class Util {
  public static ImmutableSet<Label> linesToLabels(Path path) throws IOException {
    Builder<Label> builder = new Builder<>();
    for (String line : Files.readAllLines(path, StandardCharsets.UTF_8)) {
      if (!line.isEmpty()) {
        builder.add(Label.normalize(line));
      }
    }
    return builder.build();
  }

  public static void assertTargetsMatch(
      Set<Label> targets,
      Set<String> expectedTargetStrings,
      Set<String> forbiddenTargetStrings,
      boolean allowOverBuilds) {
    Set<Label> expectedTargets =
        expectedTargetStrings.stream().map(Label::normalize).collect(Collectors.toSet());
    Set<Label> forbiddenTargets =
        forbiddenTargetStrings.stream().map(Label::normalize).collect(Collectors.toSet());

    Set<Label> missingTargets = Sets.difference(expectedTargets, targets);
    Set<Label> extraTargets = Sets.difference(targets, expectedTargets);
    Set<Label> foundForbiddenTargets = Sets.intersection(extraTargets, forbiddenTargets);

    assertThat(
        "Targets were not detected which should have been. Missing targets:",
        missingTargets,
        empty());
    assertThat(
        "Forbidden targets were additionally detected. Extra targets:",
        foundForbiddenTargets,
        empty());
    if (!allowOverBuilds && !"true".equals(System.getenv("ALLOW_OVER_BUILDING"))) {
      assertThat(
          "Extra targets were found - this isn't the end of the world, but causes over-building",
          extraTargets,
          empty());
    } else if (allowOverBuilds) {
      assertThat("allowOverBuilds is set but no over-building was done", extraTargets, not(empty()));
    }
  }

  public static TestdataRepo cloneTestdataRepo() throws Exception {
    String property = System.getProperty("target_determinator_testdata_dir");
    if (property != null) {
      return TestdataRepo.forExistingClone(Paths.get(property));
    } else {
      return TestdataRepo.create();
    }
  }
}
