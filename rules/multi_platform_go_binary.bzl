load("@io_bazel_rules_go//go:def.bzl", "go_binary")

_PLATFORMS = [
    ("darwin", "amd64"),
    ("darwin", "arm64"),
    ("linux", "amd64"),
    ("linux", "arm64"),
    ("windows", "amd64"),
]

def multi_platform_go_binary(name, **kwargs):
    if "visibility" not in kwargs:
        kwargs["visibility"] = "//visibility:public"

    if "goos" in kwargs or "goarch" in kwargs:
        fail("Can't specify goos or goarch for multi_platform_go_binary")

    go_binary(
        name = name,
        **kwargs
    )

    for goos, goarch in _PLATFORMS:
        go_binary(
            name = "{}.{}.{}".format(name, goos, goarch),
            goos = goos,
            goarch = goarch,
            **kwargs
        )
