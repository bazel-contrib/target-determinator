package pkg

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/bazel-contrib/target-determinator/common/versions"
	"github.com/hashicorp/go-version"
)

type BazelCmdConfig struct {
	// Dir represents the working directory to use for the command.
	// If Dir is the empty string, use the calling process's current directory.
	Dir string

	// Stdout and Stderr specify the process's standard output and error.
	// A nil value redirects the output to /dev/null.
	// The behavior is the same as the exec.Command struct.
	Stdout io.Writer
	Stderr io.Writer
}

type BazelCmd interface {
	Execute(config BazelCmdConfig, startupArgs []string, command string, args ...string) (int, error)
	Cquery(bazelRelease string, config BazelCmdConfig, startupArgs []string, args ...string) (int, error)
}

type DefaultBazelCmd struct {
	BazelPath        string
	BazelStartupOpts []string
	BazelOpts        []string
}

// Commands which we should apply BazelOpts to.
// This is an incomplete list, but includes all of the commands we actually use in the target determinator.
var _buildLikeCommands = map[string]struct{}{
	"build":  {},
	"config": {},
	"cquery": {},
	"test":   {},
}

// Execute calls bazel with the provided arguments.
// It returns the exit status code or -1 if it errored before the process could start.
func (c DefaultBazelCmd) Execute(config BazelCmdConfig, startupArgs []string, command string, args ...string) (int, error) {
	bazelArgv := make([]string, 0, len(c.BazelStartupOpts)+len(args))
	bazelArgv = append(bazelArgv, c.BazelStartupOpts...)
	bazelArgv = append(bazelArgv, startupArgs...)
	bazelArgv = append(bazelArgv, command)
	if _, ok := _buildLikeCommands[command]; ok {
		bazelArgv = append(bazelArgv, c.BazelOpts...)
	}
	bazelArgv = append(bazelArgv, args...)
	cmd := exec.Command(c.BazelPath, bazelArgv...)
	cmd.Dir = config.Dir
	cmd.Stdout = config.Stdout
	cmd.Stderr = config.Stderr

	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return exitError.ExitCode(), err
		} else {
			return -1, err
		}
	}
	return 0, nil
}

// Cquery calls bazel cquery with the provided arguments, using an output file if supported.
// It returns the exit status code or -1 if it errored before the process could start.
func (c DefaultBazelCmd) Cquery(bazelRelease string, config BazelCmdConfig, startupArgs []string, args ...string) (int, error) {
	hasOutputFile, _ := versions.ReleaseIsInRange(bazelRelease, version.Must(version.NewVersion("8.2.0")), nil)
	if hasOutputFile == nil || !*hasOutputFile {
		// --output_file is not supported (or we weren't able to tell), so we let cquery print to stdout.
		return c.Execute(config, startupArgs, "cquery", args...)
	}

	cqueryOutputFile, err := os.CreateTemp("", "target-determinator-cquery-*.proto")
	if err != nil {
		return 1, fmt.Errorf("failed to create temporary file for cquery output: %w", err)
	}
	cqueryOutput := cqueryOutputFile.Name()
	defer os.Remove(cqueryOutput)
	err = cqueryOutputFile.Close()
	if err != nil {
		return 1, fmt.Errorf("failed to close temporary file for cquery output: %w", err)
	}

	exitCode, err := c.Execute(config, startupArgs, "cquery", append(args, "--output_file="+cqueryOutput)...)

	cqueryOutputFile, err = os.Open(cqueryOutput)
	if err != nil {
		return exitCode, fmt.Errorf("failed to open cquery output file %s for reading: %w", cqueryOutput, err)
	}
	defer cqueryOutputFile.Close()
	if _, err = io.Copy(config.Stdout, cqueryOutputFile); err != nil {
		return exitCode, fmt.Errorf("failed to read cquery output: %w", err)
	}

	return exitCode, err
}
