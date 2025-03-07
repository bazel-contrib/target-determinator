package example.simple;

public class SubmoduleSimple {
  public static String description() {
    return String.format("I am a simple class named SubmoduleSimple, and I know about the Simple class whose description is: %s", Simple.description());
  }
}
