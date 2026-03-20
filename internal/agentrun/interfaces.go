package agentrun

import (
	"context"
	"io"
	"time"
)

type ProcessHandle interface {
	Stdout() io.Reader
	Stderr() io.Reader
	Wait() error
	Kill() error
	PID() int
}

type Adapter interface {
	ID() string
	Start(ctx context.Context, spec RunSpec) (ProcessHandle, RunRecord, error)
}

type Monitor interface {
	Observe(ctx context.Context, input ObservationInput) (Observation, error)
}

type Validator interface {
	Validate(ctx context.Context, input ValidationInput) (ValidationResult, error)
	Finalize(ctx context.Context, input ValidationInput) (ValidationResult, error)
}

type Selector interface {
	Next(ctx context.Context) (Selection, error)
}

type Orchestrator interface {
	RunTicket(ctx context.Context, ticketID string) (TicketRunSummary, error)
	RunNext(ctx context.Context) (CycleSummary, error)
	ResumeTicket(ctx context.Context, ticketID string) (TicketRunSummary, error)
}

type Observation struct {
	Result   Result        `json:"result,omitempty"`
	Review   *ReviewResult `json:"review,omitempty"`
	TimedOut bool          `json:"timed_out,omitempty"`
}

type ObservationInput struct {
	Handle  ProcessHandle `json:"-"`
	Record  RunRecord     `json:"record"`
	Timeout time.Duration `json:"timeout"`
}

type ValidationInput struct {
	TicketID     string `json:"ticket_id"`
	RepoRoot     string `json:"repo_root"`
	WorktreePath string `json:"worktree_path"`
	Branch       string `json:"branch"`
	Result       Result `json:"result"`
}

type ValidationResult struct {
	Accepted bool     `json:"accepted"`
	Reasons  []string `json:"reasons,omitempty"`
}

type Selection struct {
	TicketID string `json:"ticket_id"`
	Reason   string `json:"reason,omitempty"`
	Found    bool   `json:"found"`
}

type TicketRunSummary struct {
	TicketID string `json:"ticket_id"`
	Status   Status `json:"status"`
	Reason   string `json:"reason,omitempty"`
}

type CycleSummary struct {
	Runs       []TicketRunSummary `json:"runs,omitempty"`
	StopReason string             `json:"stop_reason,omitempty"`
}
