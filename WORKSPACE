workspace(name = "target-determinator")

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
    sha256 = "f74c98d6df55217a36859c74b460e774abc0410a47cc100d822be34d5f990f16",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/rules_go/releases/download/v0.47.1/rules_go-v0.47.1.zip",
        "https://github.com/bazelbuild/rules_go/releases/download/v0.47.1/rules_go-v0.47.1.zip",
    ],
)

load("@io_bazel_rules_go//go:deps.bzl", "go_register_toolchains", "go_rules_dependencies")

go_rules_dependencies()

go_register_toolchains(version = "1.18")

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies", "go_repository")

gazelle_dependencies()

# Pull in bazel_diff for testing.
http_file(
    name = "bazel_diff",
    downloaded_file_path = "bazel-diff_deploy.jar",
    sha256 = "5f36e74a8d6167e4d31f663526a63be3e3728456d8b5b4a84503315dd10e65e7",
    url = "https://github.com/Tinder/bazel-diff/releases/download/9.0.0/bazel-diff_deploy.jar",
)

RULES_JVM_EXTERNAL_TAG = "6.7"

RULES_JVM_EXTERNAL_SHA = "a1e351607f04fed296ba33c4977d3fe2a615ed50df7896676b67aac993c53c18"

http_archive(
    name = "rules_jvm_external",
    sha256 = RULES_JVM_EXTERNAL_SHA,
    strip_prefix = "rules_jvm_external-%s" % RULES_JVM_EXTERNAL_TAG,
    url = "https://github.com/bazel-contrib/rules_jvm_external/releases/download/%s/rules_jvm_external-%s.tar.gz" % (RULES_JVM_EXTERNAL_TAG, RULES_JVM_EXTERNAL_TAG),
)

load("@rules_jvm_external//:repositories.bzl", "rules_jvm_external_deps")

rules_jvm_external_deps()

load("@rules_jvm_external//:setup.bzl", "rules_jvm_external_setup")

rules_jvm_external_setup()

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

load("@maven//:defs.bzl", "pinned_maven_install")

pinned_maven_install()

# We need a modern `rules_python` so that the old version of `rules_proto` we have works with recent bazel releases

http_archive(
    name = "rules_python",
    sha256 = "62ddebb766b4d6ddf1712f753dac5740bea072646f630eb9982caa09ad8a7687",
    strip_prefix = "rules_python-0.39.0",
    url = "https://github.com/bazelbuild/rules_python/releases/download/0.39.0/rules_python-0.39.0.tar.gz",
)

load("@rules_python//python:repositories.bzl", "py_repositories")

py_repositories()

http_archive(
    name = "rules_proto",
    sha256 = "dc3fb206a2cb3441b485eb1e423165b231235a1ea9b031b4433cf7bc1fa460dd",
    strip_prefix = "rules_proto-5.3.0-21.7",
    urls = [
        "https://github.com/bazelbuild/rules_proto/archive/refs/tags/5.3.0-21.7.tar.gz",
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
    executable = True,
    integrity = "sha256-eFjQ2D6auwcnycoY67qOx6NJPsI2ZKSUv1cPdaBVtOo=",
    url = "https://github.com/ewhauser/bazel-differ/releases/download/v0.0.7/bazel-differ-linux-arm64",
)

http_file(
    name = "bazel_differ_linux_x86_64",
    executable = True,
    integrity = "sha256-quwSTcr6dHF0Jh7JyCR74zMsItHRcS7YD7YSAClp5CA=",
    url = "https://github.com/ewhauser/bazel-differ/releases/download/v0.0.7/bazel-differ-linux-x86_64",
)

http_file(
    name = "bazel_differ_darwin_arm64",
    executable = True,
    integrity = "sha256-0dbJKJXHzTr0/43nJxFO+xGbCPiGYEqVirve25SXss4=",
    url = "https://github.com/ewhauser/bazel-differ/releases/download/v0.0.7/bazel-differ-darwin-arm64",
)

http_file(
    name = "bazel_differ_darwin_x86_64",
    executable = True,
    integrity = "sha256-wS/sbX/XgsIaX51VUOGyv7wRzKkmOUgLHzx+wg2weVE=",
    url = "https://github.com/ewhauser/bazel-differ/releases/download/v0.0.7/bazel-differ-darwin-x86_64",
)
