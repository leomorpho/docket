package semantic

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

type CommandSpec struct {
	Path  string
	Args  []string
	Env   []string
	Dir   string
	Stdin []byte
}

type CommandResult struct {
	Stdout []byte
	Stderr []byte
}

type Runner interface {
	Run(context.Context, CommandSpec) (CommandResult, error)
}

type ExecRunner struct{}

type CommandError struct {
	Spec   CommandSpec
	Stderr string
	Err    error
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("run %s: %v", e.Spec.Path, e.Err)
}

func (r ExecRunner) Run(ctx context.Context, spec CommandSpec) (CommandResult, error) {
	cmd := exec.CommandContext(ctx, spec.Path, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	cmd.Stdin = bytes.NewReader(spec.Stdin)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, &CommandError{
			Spec:   spec,
			Stderr: stderr.String(),
			Err:    err,
		}
	}
	return CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, nil
}
