package codex

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/leomorpho/docket/internal/agentrun"
)

type commandFactory func(ctx context.Context, name string, args ...string) *exec.Cmd

type Runner struct {
	command string
	newCmd  commandFactory
	now     func() time.Time
}

func NewRunner() *Runner {
	return &Runner{
		command: "codex",
		newCmd:  exec.CommandContext,
		now:     time.Now,
	}
}

func (r *Runner) ID() string {
	return "codex"
}

func (r *Runner) Start(ctx context.Context, spec agentrun.RunSpec) (agentrun.ProcessHandle, agentrun.RunRecord, error) {
	if err := spec.Validate(); err != nil {
		return nil, agentrun.RunRecord{}, err
	}

	sessionID := fmt.Sprintf("%s-%d", spec.TicketID, r.now().UTC().UnixNano())
	cmd := r.newCmd(
		ctx,
		r.command,
		"exec",
		"--json",
		"--skip-git-repo-check",
		"--ephemeral",
		"--dangerously-bypass-approvals-and-sandbox",
		"-C",
		spec.WorktreePath,
		"-",
	)
	cmd.Env = append(os.Environ(),
		"DOCKET_SESSION_ID="+sessionID,
		"DOCKET_TICKET_ID="+spec.TicketID,
		"DOCKET_RUN_ROLE="+string(spec.Role),
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, agentrun.RunRecord{}, fmt.Errorf("codex stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, agentrun.RunRecord{}, fmt.Errorf("codex stderr pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, agentrun.RunRecord{}, fmt.Errorf("codex stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, agentrun.RunRecord{}, fmt.Errorf("start codex exec: %w", err)
	}
	if _, err := io.WriteString(stdin, spec.Prompt); err != nil {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		return nil, agentrun.RunRecord{}, fmt.Errorf("write codex prompt: %w", err)
	}
	if err := stdin.Close(); err != nil {
		_ = cmd.Process.Kill()
		return nil, agentrun.RunRecord{}, fmt.Errorf("close codex prompt: %w", err)
	}

	record := agentrun.RunRecord{
		TicketID:     spec.TicketID,
		Role:         spec.Role,
		Adapter:      r.ID(),
		RepoRoot:     spec.RepoRoot,
		WorktreePath: spec.WorktreePath,
		Branch:       spec.Branch,
		StartedAt:    r.now().UTC().Format(time.RFC3339Nano),
		SessionID:    sessionID,
	}
	return processHandle{cmd: cmd, stdout: stdout, stderr: stderr}, record, nil
}

type processHandle struct {
	cmd    *exec.Cmd
	stdout io.Reader
	stderr io.Reader
}

func (h processHandle) Stdout() io.Reader { return h.stdout }
func (h processHandle) Stderr() io.Reader { return h.stderr }
func (h processHandle) Wait() error       { return h.cmd.Wait() }
func (h processHandle) Kill() error {
	if h.cmd == nil || h.cmd.Process == nil {
		return nil
	}
	return h.cmd.Process.Kill()
}
func (h processHandle) PID() int {
	if h.cmd == nil || h.cmd.Process == nil {
		return 0
	}
	return h.cmd.Process.Pid
}
