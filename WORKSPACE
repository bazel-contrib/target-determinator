load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive", "http_file")

http_archive(
    name = "bazel_gazelle",
    sha256 = "5d80e62a70314f39cc764c1c3eaa800c5936c9f1ea91625006227ce4d20cd086",
    urls = [
        "https://github.com/bazelbuild/bazel-gazelle/releases/download/v0.42.0/bazel-gazelle-v0.42.0.tar.gz",
    ],
)

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "099a9fb96a376ccbbb7d291ed4ecbdfd42f6bc822ab77ae6f1b5cb9e914e94fa",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/rules_go/releases/download/v0.35.0/rules_go-v0.35.0.zip",
        "https://github.com/bazelbuild/rules_go/releases/download/v0.35.0/rules_go-v0.35.0.zip",
    ],
)

load("@io_bazel_rules_go//go:deps.bzl", "go_register_toolchains", "go_rules_dependencies")

go_rules_dependencies()

go_register_toolchains(version = "1.18")

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies", "go_repository")

gazelle_dependencies()

# Pull in bazel_diff for testing.
http_archive(
    name = "bazel_diff",
    patch_args = ["-p1"],
    patches = ["@//:third_party/patches/bazel-diff-only-just-non-external-rules.patch"],
    sha256 = "bdc3ef2192e9aeb288506e2f348aef57bce5e4facf7374b6185ac5ccdd4a9001",
    strip_prefix = "bazel-diff-3.5.0",
    url = "https://github.com/Tinder/bazel-diff/archive/refs/tags/3.5.0.tar.gz",
)

RULES_JVM_EXTERNAL_TAG = "4.2"

RULES_JVM_EXTERNAL_SHA = "2cd77de091e5376afaf9cc391c15f093ebd0105192373b334f0a855d89092ad5"

http_archive(
    name = "rules_jvm_external",
    sha256 = RULES_JVM_EXTERNAL_SHA,
    strip_prefix = "rules_jvm_external-%s" % RULES_JVM_EXTERNAL_TAG,
    url = "https://github.com/bazelbuild/rules_jvm_external/archive/refs/tags/%s.tar.gz" % RULES_JVM_EXTERNAL_TAG,
)

load("@rules_jvm_external//:repositories.bzl", "rules_jvm_external_deps")

rules_jvm_external_deps()

load("@rules_jvm_external//:setup.bzl", "rules_jvm_external_setup")

rules_jvm_external_setup()

load("@bazel_diff//:artifacts.bzl", "BAZEL_DIFF_MAVEN_ARTIFACTS")
load("@rules_jvm_external//:defs.bzl", "maven_install")

maven_install(
    artifacts = [
        "com.google.guava:guava:31.0.1-jre",
        "junit:junit:4.12",
        "org.eclipse.jgit:org.eclipse.jgit:5.11.0.202103091610-r",
        "org.hamcrest:hamcrest-all:1.3",
    ],
    fail_if_repin_required = True,
    fetch_sources = True,
    maven_install_json = "@//:maven_install.json",
    repositories = [
        "https://repo1.maven.org/maven2",
    ],
)

maven_install(
    name = "bazel_diff_maven",
    artifacts = BAZEL_DIFF_MAVEN_ARTIFACTS,
    fail_if_repin_required = True,
    maven_install_json = "@//:bazel_diff_maven_install.json",
    repositories = [
        "https://repo1.maven.org/maven2",
    ],
)

load("@maven//:defs.bzl", "pinned_maven_install")

pinned_maven_install()

load("@bazel_diff_maven//:defs.bzl", bazel_diff_pinned_maven_install = "pinned_maven_install")

bazel_diff_pinned_maven_install()

http_archive(
    name = "rules_proto",
    sha256 = "b9e1268c5bce4bb01ef31730300b8a4f562dc1211088f125c39af716f6f65f60",
    strip_prefix = "rules_proto-e507ccded37c389186afaeb2b836ec576dc875dc",
    urls = [
        "https://github.com/bazelbuild/rules_proto/archive/e507ccded37c389186afaeb2b836ec576dc875dc.tar.gz",
    ],
)

load("@rules_proto//proto:repositories.bzl", "rules_proto_dependencies", "rules_proto_toolchains")

rules_proto_dependencies()

rules_proto_toolchains()

load("//:third_party/go/deps.bzl", "go_dependencies")

# gazelle:repository_macro third_party/go/deps.bzl%go_dependencies
go_dependencies()

##########################################################
# bazel-differ: https://github.com/ewhauser/bazel-differ #
##########################################################

http_file(
    name = "bazel_differ_linux_arm64",
    sha256 = "7a7166aa526c3688b9fb71dfc8913c35902d4a440a32ff02db55e7b91a97f666",
    url = "https://github.com/ewhauser/bazel-differ/releases/download/v0.0.5/bazel-differ-linux-arm64",
)

http_file(
    name = "bazel_differ_linux_x86_64",
    sha256 = "ce563301a22f41cc111f2966fc77a23e655fbe1140d93b33eb4818f06c40a8d5",
    url = "https://github.com/ewhauser/bazel-differ/releases/download/v0.0.5/bazel-differ-linux-x86_64",
)

http_file(
    name = "bazel_differ_darwin_arm64",
    sha256 = "0bcbfecfb9788764efa63922322b3c2d5a1a9c0da26b2dd8e7565ca60960e231",
    url = "https://github.com/ewhauser/bazel-differ/releases/download/v0.0.5/bazel-differ-darwin-arm64",
)

http_file(
    name = "bazel_differ_darwin_x86_64",
    sha256 = "a18c17f1a5e9830dfadb5ad3df02b453882fc9ec46837e9f8450c655d2cbe214",
    url = "https://github.com/ewhauser/bazel-differ/releases/download/v0.0.5/bazel-differ-darwin-x86_64",
)
