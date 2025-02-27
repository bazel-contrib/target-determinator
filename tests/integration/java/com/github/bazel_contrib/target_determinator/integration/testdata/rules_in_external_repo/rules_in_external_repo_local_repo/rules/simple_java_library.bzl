JAVAC_CMD = "javac"
JAR_CMD = "jar"

def _simple_java_library(ctx):
    out_classdir = ctx.actions.declare_directory("{}.classdir".format(ctx.attr.name))
    out_jar = ctx.actions.declare_file("{}.jar".format(ctx.attr.name))

    dep_java_infos = [dep[JavaInfo] for dep in ctx.attr.deps]
    dep_jars = depset(transitive = [dep.compile_jars for dep in dep_java_infos])

    ctx.actions.run_shell(
        command = "{} -cp {} {} -d {}".format(JAVAC_CMD, ":".join(["."] + [dep.path for dep in dep_jars.to_list()]), ctx.file.src.path, out_classdir.path),
        inputs = depset(direct = [ctx.file.src], transitive = [dep_jars]),
        outputs = [out_classdir],
        mnemonic = "SimpleJavac",
    )

    ctx.actions.run_shell(
        command = "{} cf $(pwd)/{} -C {} .".format(JAR_CMD, out_jar.path, out_classdir.path),
        inputs = [out_classdir],
        outputs = [out_jar],
        mnemonic = "JarClasses",
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
