package pkg

import (
	"io"
	"os/exec"
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
