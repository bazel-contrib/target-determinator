load("@rules//rules:simple_java_library.bzl", _simple_java_library = "simple_java_library")

def simple_java_library(name, src, deps = None, visibility = None):
    _simple_java_library(
        name = name,
        src = src,
        deps = deps,
        visibility = visibility,
        source = "7",
        target = "7",
    )
