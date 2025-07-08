package pkg

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/bazel-contrib/target-determinator/common/versions"
	"github.com/hashicorp/go-version"
)

func BazelOutputBase(workingDirectory string, BazelCmd BazelCmd) (string, error) {
	return bazelInfo(workingDirectory, BazelCmd, "output_base")
}

func BazelRelease(workingDirectory string, BazelCmd BazelCmd) (string, error) {
	return bazelInfo(workingDirectory, BazelCmd, "release")
}

func bazelInfo(workingDirectory string, bazelCmd BazelCmd, key string) (string, error) {
	var stdoutBuf, stderrBuf bytes.Buffer

	result, err := bazelCmd.Execute(
		BazelCmdConfig{Dir: workingDirectory, Stdout: &stdoutBuf, Stderr: &stderrBuf},
		nil, "info", key)

	if result != 0 || err != nil {
		return "", fmt.Errorf("failed to get the Bazel %v: %w. Stderr:\n%v", key, err, stderrBuf.String())
	}
	return strings.TrimRight(stdoutBuf.String(), "\n"), nil
}

func IsBzlmodEnabled(workspacePath string, bazelCmd BazelCmd, releaseString string) (bool, error) {
	semanticsStr, err := bazelInfo(workspacePath, bazelCmd, "starlark-semantics")
	if err != nil {
		return false, fmt.Errorf("failed to get Bazel starlark-semantics info: %w", err)
	}

	// Check if enable_bzlmod is explicitly set
	enableBzlmodRegex := regexp.MustCompile(`enable_bzlmod=(true|false)`)
	matches := enableBzlmodRegex.FindStringSubmatch(semanticsStr)

	if len(matches) > 1 {
		// If explicitly set, return the value
		return matches[1] == "true", nil
	}

	// If not explicitly set, determine based on Bazel version
	// In Bazel 6.x, the default is false
	// In Bazel 7.x and later, the default is true
	isAtLeastBazel7, _ := versions.ReleaseIsInRange(releaseString, version.Must(version.NewVersion("7.0.0")), nil)
	if isAtLeastBazel7 != nil && *isAtLeastBazel7 {
		return true, nil
	}
	return false, nil
}
