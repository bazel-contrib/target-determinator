package com.github.bazel_contrib.target_determinator.integration;

class Commits {

    public static final String NO_TARGETS = "no_targets";
    public static final String EMPTY_SUBMODULE = "empty_submodule";
    public static final String ADD_DEPENDENT_ON_SIMPLE_JAVA_LIBRARY = "add_dependent_on_simple_java_library";
    public static final String ONE_TEST = "one_test";
    public static final String ONE_TEST_BAZEL7_0_0 = "one_test_bazel_7_0_0";
    public static final String TWO_TESTS = "two_tests";
    public static final String HAS_JVM_FLAGS = "has_jvm_flags";
    public static final String EXPLICIT_DEFAULT_VALUE = "explicit_default_value";
    public static final String TWO_NATIVE_TESTS_BAZEL5_4_0 = "two_native_tests_bazel_5_4_0";
    public static final String TWO_NATIVE_TESTS_BAZEL6_0_0 = "two_native_tests_bazel_6_0_0";
    public static final String MODIFIED_TEST_SRC = "modified_test_src";
    public static final String TWO_LANGUAGES_OF_TESTS = "two_languages_of_tests";
    public static final String BAZELRC_TEST_ENV = "bazelrc_test_env";
    public static final String BAZELRC_AFFECTING_JAVA = "bazelrc_affecting_java";
    public static final String SIMPLE_TARGETS_BAZEL5_4_0 = "simple_targets_bazel_5_4_0";
    public static final String SIMPLE_TARGETS_BAZEL6_0_0 = "simple_targets_bazel_6_0_0";
    public static final String ADD_OPTIONAL_PRESENT_EMPTY_BAZELRC = "add_optional_present_empty_bazelrc";
    public static final String SIMPLE_JAVA_LIBRARY_RULE = "simple_java_library_rule";
    public static final String SIMPLE_JAVA_LIBRARY_TARGETS = "simple_java_library_targets";
    public static final String SIMPLE_JAVA_LIBRARY_AND_JAVA_TESTS = "simple_java_library_and_java_tests";
    public static final String CHANGE_TRANSITIVE_FILE = "change_transitive_file";
    public static final String CHANGE_TRANSITIVE_FILE_BAZEL4_0_0 = "change_transitive_file_bazel_4_0_0";
    public static final String TWO_LANGUAGES_OPTIONAL_MISSING_TRY_IMPORT = "two_languages_optional_missing_try_import";
    public static final String TWO_LANGUAGES_OPTIONAL_PRESENT_BAZELRC_AFFECTING_JAVA =
            "two_languages_optional_present_bazelrc_affecting_java";
    public static final String TWO_LANGUAGES_NOOP_IMPORTED_BAZELRC = "two_languages_noop_imported_bazelrc";
    public static final String TWO_LANGUAGES_IMPORTED_BAZELRC_AFFECTING_JAVA =
            "two_languages_imported_bazelrc_affecting_java";
    public static final String JAVA_TESTS_AND_SIMPLE_JAVA_RULES = "java_tests_and_simple_java_rules";
    public static final String DEP_ON_STARLARK_TARGET = "dep_on_starlark_target";
    public static final String CHANGE_STARLARK_RULE_IMPLEMENTATION = "change_starlark_rule_implementation";
    public static final String NOOP_REFACTOR_STARLARK_RULE_IMPLEMENTATION =
            "noop_refactor_starklark_rule_implementation";
    public static final String RULES_IN_EXTERNAL_REPO = "rules_in_external_repo";
    public static final String NOOP_REFACTOR_IN_WORKSPACE_FILE = "noop_refactor_in_workspace_file";
    public static final String ADD_SIMPLE_PACKAGE_RULE = "add_simple_package_rule";
    public static final String REFACTORED_WORKSPACE_INDIRECTLY =
            "refactored_workspace_indirectly";
    public static final String PATHOLOGICAL_RULES_SINGLE_TARGET =
            "pathological_rules_single_target";
    public static final String PATHOLOGICAL_RULES_TWO_TARGETS =
            "pathological_rules_two_targets";
    public static final String PATHOLOGICAL_RULES_THREE_TARGETS =
            "pathological_rules_three_targets";
    public static final String PATHOLOGICAL_RULES_FIVE_TARGETS =
            "pathological_rules_five_targets";
    public static final String CHANGE_ATTRIBUTES_VIA_INDIRECTION =
            "change_attributes_via_indirection";
    public static final String HAS_GLOBS = "has_globs";
    public static final String CHANGE_GLOBS = "change_globs";
    public static final String ADD_BUILD_FILE_INTERFERING_WTH_GLOBS = "add_build_file_interfering_with_globs";
    public static final String BAZELRC_INCLUDED_EMPTY = "bazelrc_included_empty";
    public static final String JAVA_USED_IN_GENRULE = "java_used_in_genrule";
    public static final String BAZELRC_INCLUDED_JAVACOPT = "bazelrc_included_javacopt";
    public static final String BAZELRC_HOST_JAVACOPT = "bazelrc_host_javacopt";
    public static final String ADD_INDIRECTION_FOR_SIMPLE_JAVA_LIBRARY = "add_indirection_for_simple_java_library";
    public static final String REDUCE_DEPENDENCY_VISIBILITY = "reduce_dependency_visibility";
    public static final String ONE_TEST_WITH_GITIGNORE = "one_test_with_gitignore";
    public static final String TWO_TESTS_WITH_GITIGNORE = "two_tests_with_gitignore";
    public static final String TWO_TESTS_BRANCH =
            "two-tests-branch"; // Local only (created by the test case).
    public static final String ONE_SH_TEST = "one_sh_test";
    public static final String SH_TEST_NOT_EXECUTABLE = "sh_test_not_executable";
    public static final String INCOMPATIBLE_TARGET = "incompatible_target";
    public static final String INCOMPATIBLE_TARGET_BAZEL7_0_0 = "incompatible_target_bazel_7_0_0";
    public static final String SELECT_TARGET = "select_target";

    public static final String CHANGED_NONLINUX_SRC = "changed_nonlinux_src";

    public static final String CHANGED_LINUX_SRC = "changed_linux_src";

    public static final String CHANGED_NONLINUX_DEP = "changed_nonlinux_dep";

    public static final String CHANGED_LINUX_DEP = "changed_linux_dep";

    public static final String ALIAS_ADD_TARGET = "alias_add_target";

    public static final String ALIAS_CHANGE_ACTUAL = "alias_change_actual";

    public static final String ALIAS_CHANGE_TARGET_THROUGH_ALIAS = "alias_change_target_through_alias";

    public static final String ALIAS_ADD_TARGET_TO_FILE = "alias_add_target_to_file";

    public static final String ALIAS_CHANGE_TARGET_THROUGH_ALIAS_TO_FILE = "alias_change_target_through_alias_to_file";
}
