load("@bazel_gazelle//:deps.bzl", _go_repository = "go_repository")

def go_repository(**kwargs):
    if "build_external" in kwargs:
        fail("Saw build_external in go_repository shim kwargs")

    # Work around https://github.com/bazelbuild/bazel-gazelle/issues/1344
    kwargs["build_external"] = "external"
    _go_repository(**kwargs)
