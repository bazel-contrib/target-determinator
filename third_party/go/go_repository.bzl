load("@bazel_gazelle//:deps.bzl", _go_repository = "go_repository")

def go_repository(**kwargs):
    if "build_external" in kwargs:
        fail("Saw build_external in go_repository shim kwargs")

    # Work around https://github.com/bazelbuild/bazel-gazelle/issues/1344
    kwargs["build_external"] = "external"

    # Work around for https://github.com/bazel-contrib/bazel-gazelle/pull/2021 until new release of bazel-gazelle
    if kwargs["importpath"] == "github.com/bazelbuild/bazel-gazelle":
        kwargs["patches"] = [
            "//third_party/patches:gazelle_regexp.patch",
        ]

    _go_repository(**kwargs)
