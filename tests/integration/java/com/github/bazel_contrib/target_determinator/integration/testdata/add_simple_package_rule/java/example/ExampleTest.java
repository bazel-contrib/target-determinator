package example;

import example.simple.Simple;

import org.junit.Assert;
import org.junit.Test;

public class ExampleTest {
  @Test
  public void passes() {
    Assert.assertEquals("I am a simple class named Simple, and I know about Dep", Simple.description());
  }
}
