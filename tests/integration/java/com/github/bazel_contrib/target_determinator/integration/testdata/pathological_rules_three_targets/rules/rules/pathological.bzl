def pathological(name, srcs):
    """A dastardly Bazel macro.

    If there are an odd number of srcs, each target generates its own output.
    If there are an even number of srcs, each target generates the next src's output.
    But if there are 5 or more, just one output is generated.
    """
    l = len(srcs)
    if l >= 5:
        native.genrule(
            name = name,
            srcs = srcs,
            outs = ["{}.lengths".format(name)],
            cmd = "wc -c $(SRCS) > $@",
        )
    else:
        for i in range(len(srcs)):
            src_name = srcs[i]
            if l % 2 == 1:
                input_file = srcs[i]
            else:
                input_file = srcs[(i + 1) % l]
            native.genrule(
                name = "length_of_" + src_name,
                srcs = [input_file],
                outs = [src_name + ".length"],
                cmd = "cat $< | wc -c | xargs echo > $@",
            )
