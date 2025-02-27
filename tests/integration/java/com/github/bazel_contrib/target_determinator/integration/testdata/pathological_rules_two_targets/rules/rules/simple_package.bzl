def _simple_package(ctx):
    out = ctx.actions.declare_file("{}.tar.gz".format(ctx.attr.name))

    ctx.actions.run_shell(
        command = "gnutar --owner root --group wheel --mtime='UTC 1980-01-01' -h -cf /dev/stdout {} | gzip -n > {}".format(" ".join([str(src.path) for src in ctx.files.srcs]), out.path),
        inputs = ctx.files.srcs,
        outputs = [out],
        mnemonic = "SimplePackage",
    )
    return [
        DefaultInfo(files = depset([out])),
    ]

simple_package = rule(
    implementation = _simple_package,
    attrs = {
        "srcs": attr.label_list(allow_files = True),
    },
)
