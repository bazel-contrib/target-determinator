load("@bazel_tools//tools/build_defs/repo:git.bzl", "git_repository")
load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "bazel_gazelle",
    patch_args = ["-p1"],
    # Pull in https://github.com/bazelbuild/bazel-gazelle/pull/1227
    patches = ["@//:third_party/patches/bazel_gazelle/label-pattern-matching.patch"],
    sha256 = "b751f7fa79829a06778e91cb721e2bcd1e7251d9b22eb8d9ebc4993ecb3ef8dc",
    strip_prefix = "bazel-gazelle-bd319f810c16ba206a2b87422e8d328cefaded88",
    urls = [
        "https://github.com/bazelbuild/bazel-gazelle/archive/bd319f810c16ba206a2b87422e8d328cefaded88.zip",
    ],
)

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "f2dcd210c7095febe54b804bb1cd3a58fe8435a909db2ec04e31542631cf715c",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/rules_go/releases/download/v0.31.0/rules_go-v0.31.0.zip",
        "https://github.com/bazelbuild/rules_go/releases/download/v0.31.0/rules_go-v0.31.0.zip",
    ],
)

load("@io_bazel_rules_go//go:deps.bzl", "go_register_toolchains", "go_rules_dependencies")

go_rules_dependencies()

go_register_toolchains(version = "1.18")

load("//:third_party/go/deps.bzl", "go_dependencies")

# gazelle:repository_macro third_party/go/deps.bzl%go_dependencies
go_dependencies()

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies", "go_repository")

gazelle_dependencies()

# Pull in bazel_diff for testing.
git_repository(
    name = "bazel_diff",
    commit = "64ec2868bf58bb7b02d2e045946d3b65c3df5197",
    patch_args = ["-p1"],
    patches = ["@//:third_party/patches/bazel-diff-only-just-non-external-rules.patch"],
    remote = "https://github.com/Tinder/bazel-diff.git",
)

RULES_JVM_EXTERNAL_TAG = "4.2"

RULES_JVM_EXTERNAL_SHA = "cd1a77b7b02e8e008439ca76fd34f5b07aecb8c752961f9640dea15e9e5ba1ca"

http_archive(
    name = "rules_jvm_external",
    sha256 = RULES_JVM_EXTERNAL_SHA,
    strip_prefix = "rules_jvm_external-%s" % RULES_JVM_EXTERNAL_TAG,
    url = "https://github.com/bazelbuild/rules_jvm_external/archive/%s.zip" % RULES_JVM_EXTERNAL_TAG,
)

load("@rules_jvm_external//:repositories.bzl", "rules_jvm_external_deps")

rules_jvm_external_deps()

load("@rules_jvm_external//:setup.bzl", "rules_jvm_external_setup")

rules_jvm_external_setup()

load("@rules_jvm_external//:defs.bzl", "maven_install")
load("@bazel_diff//:artifacts.bzl", "BAZEL_DIFF_MAVEN_ARTIFACTS")

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

# Pull in bazel_differ for testing
git_repository(
    name = "bazel_differ",
    commit = "e5e6ac7bc13643f7135df415fa364b8d9a6935cc",
    remote = "https://github.com/ewhauser/bazel-differ.git",
)

load("@bazel_differ//:deps.bzl", bazel_differ_deps = "go_dependencies")

# Unfortunately if we don't vendor this file into the repo, gazelle doesn't seem to properly handle its contents.
# gazelle:repository_macro third_party/go/bazel_differ_deps.bzl%go_dependencies
# gazelle:ignore
bazel_differ_deps()

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
