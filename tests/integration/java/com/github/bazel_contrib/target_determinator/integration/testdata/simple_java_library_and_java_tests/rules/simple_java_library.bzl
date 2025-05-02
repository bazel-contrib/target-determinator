def _simple_java_library(ctx):
    out_jar = ctx.actions.declare_file("{}.jar".format(ctx.attr.name))

    dep_java_infos = [dep[JavaInfo] for dep in ctx.attr.deps]
    dep_jars = depset(transitive = [dep.compile_jars for dep in dep_java_infos])

    ctx.actions.run_shell(
        command = "dir=\"$(mktemp -d)\" ; javac -cp {} {} -d \"${{dir}}\" && jar cf $(pwd)/{} -C ${{dir}} .".format(":".join(["."] + [dep.path for dep in dep_jars.to_list()]), ctx.file.src.path, out_jar.path),
        inputs = depset(direct = [ctx.file.src], transitive = [dep_jars]),
        outputs = [out_jar],
        mnemonic = "SimpleJavac",
    )

    return [
        DefaultInfo(files = depset([out_jar])),
        JavaInfo(output_jar=out_jar, compile_jar=out_jar, deps=dep_java_infos),
    ]

simple_java_library = rule(
    implementation = _simple_java_library,
    attrs = {
        "src": attr.label(allow_single_file = [".java"]),
        "deps": attr.label_list(providers = [JavaInfo]),
    },
)
