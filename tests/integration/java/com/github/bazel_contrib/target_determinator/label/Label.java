package com.github.bazel_contrib.target_determinator.label;

import java.util.Objects;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/**
 * Label represents a Bazel target Label as described at https://bazel.build/concepts/labels
 *
 * <p>This is a very poor implementation of a Label, as we really only need basic normalization in
 * order to interpret results from target determinators. The only normalization performed is that
 * //foo will be normalized to //foo:foo.
 */
public class Label {
  private static final Pattern PATTERN =
      Pattern.compile("(@(?<repo>[^/]*))?//(?<package>[^:]*)(?<colonandname>:(?<name>.*))?");

  private final String normalized;

  private Label(String normalized) {
    this.normalized = normalized;
  }

  public static Label normalize(String raw) {
    Matcher matcher = PATTERN.matcher(raw);
    if (!matcher.matches()) {
      throw new IllegalArgumentException("Illegal label: " + raw);
    }
    if (matcher.group("colonandname") != null) {
      return new Label(raw);
    }
    String packageMatch = matcher.group("package");
    if (packageMatch == null || "".equals(packageMatch)) {
      throw new IllegalStateException("Empty package names not supported");
    }
    String[] packageParts = packageMatch.split("/");
    if ("...".equals(packageParts[packageParts.length - 1])) {
      return new Label(raw);
    }
    return new Label(raw + ":" + packageParts[packageParts.length - 1]);
  }

  @Override
  public String toString() {
    return normalized;
  }

  @Override
  public boolean equals(Object o) {
    if (this == o) {
      return true;
    }
    if (o == null || getClass() != o.getClass()) {
      return false;
    }
    Label label = (Label) o;
    return normalized.equals(label.normalized);
  }

  @Override
  public int hashCode() {
    return Objects.hash(normalized);
  }
}
