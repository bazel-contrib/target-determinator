load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive", "http_file")

def _non_module_deps_impl(ctx):
    http_file(
        name = "bazel-diff",
        downloaded_file_path = "bazel-diff.jar",
        executable = True,
        sha256 = "5f36e74a8d6167e4d31f663526a63be3e3728456d8b5b4a84503315dd10e65e7",
        urls = [
            "https://github.com/Tinder/bazel-diff/releases/download/9.0.0/bazel-diff_deploy.jar",
        ],
    )

extension = module_extension(implementation = _non_module_deps_impl)
