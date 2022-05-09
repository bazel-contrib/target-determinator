def _copy_proto_output_impl(ctx):
    runner_script = ctx.actions.declare_file("{}.copy_proto_output.sh".format(ctx.attr.name))

    generated = ctx.attr.proto_library[OutputGroupInfo].go_generated_srcs

    srcs = " ".join([f.short_path for f in generated.to_list()])

    ctx.actions.write(
        output = runner_script,
        content = """#!/bin/bash
out_dir="${{BUILD_WORKSPACE_DIRECTORY}}/{package}"
cp -f {srcs} "${{out_dir}}/"
echo "Copied Go generated for protos to ${{out_dir}}"
""".format(
            srcs = srcs,
            package = ctx.label.package,
        ),
        is_executable = True,
    )

    runfiles = ctx.runfiles([runner_script], transitive_files = generated)

    return [
        DefaultInfo(
            files = depset([runner_script]),
            runfiles = runfiles,
            executable = runner_script,
        ),
    ]

copy_proto_output = rule(
    executable = True,
    implementation = _copy_proto_output_impl,
    attrs = {
        "proto_library": attr.label(allow_files = True),
    },
)
