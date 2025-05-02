package example.simple;

public class Simple {
  public static String description() {
    return String.format("I am a simple class named Simple, and I know about %s", Dep.NAME);
  }
}
