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
	command   string
	newCmd    commandFactory
	now       func() time.Time
	ephemeral bool
	adapterID string
}

func NewRunner() *Runner {
	return &Runner{
		command:   "codex",
		newCmd:    exec.CommandContext,
		now:       time.Now,
		ephemeral: true,
		adapterID: "codex",
	}
}

func NewSessionRunner() *Runner {
	return &Runner{
		command:   "codex",
		newCmd:    exec.CommandContext,
		now:       time.Now,
		ephemeral: false,
		adapterID: "codex-session",
	}
}

func (r *Runner) ID() string {
	return r.adapterID
}

func (r *Runner) Start(ctx context.Context, spec agentrun.RunSpec) (agentrun.ProcessHandle, agentrun.RunRecord, error) {
	return r.start(ctx, spec, "")
}

func (r *Runner) Resume(ctx context.Context, sessionID string, spec agentrun.RunSpec) (agentrun.ProcessHandle, agentrun.RunRecord, error) {
	if r.ephemeral {
		return nil, agentrun.RunRecord{}, fmt.Errorf("runner %s does not support resume", r.ID())
	}
	if sessionID == "" {
		return nil, agentrun.RunRecord{}, fmt.Errorf("session id is required")
	}
	return r.start(ctx, spec, sessionID)
}

func (r *Runner) start(ctx context.Context, spec agentrun.RunSpec, resumeSessionID string) (agentrun.ProcessHandle, agentrun.RunRecord, error) {
	if err := spec.Validate(); err != nil {
		return nil, agentrun.RunRecord{}, err
	}

	sessionID := fmt.Sprintf("%s-%d", spec.TicketID, r.now().UTC().UnixNano())
	if resumeSessionID != "" {
		sessionID = resumeSessionID
	}
	args := []string{
		"-C",
		spec.WorktreePath,
		"exec",
	}
	if resumeSessionID != "" {
		args = append(args, "resume")
	}
	args = append(args,
		"--json",
		"--skip-git-repo-check",
		"--dangerously-bypass-approvals-and-sandbox",
	)
	if r.ephemeral {
		args = append(args, "--ephemeral")
	}
	if resumeSessionID != "" {
		args = append(args, resumeSessionID)
	}
	args = append(args, "-")
	cmd := r.newCmd(ctx, r.command, args...)
	cmd.Dir = spec.WorktreePath
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
