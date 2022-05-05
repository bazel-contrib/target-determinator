package com.github.bazel_contrib.target_determinator.label;

import static org.hamcrest.MatcherAssert.assertThat;
import static org.hamcrest.core.IsEqual.equalTo;

import org.junit.Test;

public class LabelTest {
  @Test
  public void normalizesShortWithoutRepo() {
    assertThat(Label.normalize("//foo").toString(), equalTo("//foo:foo"));
  }

  @Test
  public void normalizesLongWithoutRepo() {
    assertThat(Label.normalize("//foo:foo").toString(), equalTo("//foo:foo"));
  }

  @Test
  public void normalizesDifferentNameWithoutRepo() {
    assertThat(Label.normalize("//foo:bar").toString(), equalTo("//foo:bar"));
  }

  @Test
  public void normalizesDotDotDotWithoutRepo() {
    assertThat(Label.normalize("//...").toString(), equalTo("//..."));
  }

  @Test
  public void normalizesShortWithRepo() {
    assertThat(Label.normalize("@repo//foo").toString(), equalTo("@repo//foo:foo"));
  }

  @Test
  public void normalizesLongWithRepo() {
    assertThat(Label.normalize("@repo//foo:foo").toString(), equalTo("@repo//foo:foo"));
  }

  @Test
  public void normalizesDifferentNameWithRepo() {
    assertThat(Label.normalize("@repo//foo:bar").toString(), equalTo("@repo//foo:bar"));
  }
}
