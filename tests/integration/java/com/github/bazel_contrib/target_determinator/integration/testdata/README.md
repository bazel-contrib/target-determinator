This directory contains test data to be used by the integration tests. Each
sub-directory contains a complete copy of a repo, as if you'd just executed
`git checkout` to some shasum (minus the `.git` directory).

Note that because we include the testdata within the file tree, all build files
within the testdata are called `BUILD.bazel.mv`. When these are copied into 
during the test, these are renamed `BUILD.bazel`. This allows us to glob all 
the testdata in the regular build.

The tests can access this test data using the `TestRepo` class