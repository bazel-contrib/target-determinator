import java.util.Arrays;

public class JBin {
  public static void main(String[] args) {
    Arrays.asList(args).stream().map(String::toUpperCase).forEach(System.out::println);
  }
}
