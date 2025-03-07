load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

def setup():
    RULES_GIT_TAG = "rules/v0/pathological"
    RULES_ARCHIVE_SHA256 = "8cb9c0dfa265f6ba4378076a859380e275cccd6eda5e6d0f6b43b98259d38f8b"

    http_archive(
        name = "rules",
        sha256 = RULES_ARCHIVE_SHA256,
        strip_prefix = "target-determinator-testdata-" + RULES_GIT_TAG.replace("/", "-"),
        url = "https://github.com/bazel-contrib/target-determinator-testdata/archive/refs/tags/" + RULES_GIT_TAG + ".tar.gz",
    )
