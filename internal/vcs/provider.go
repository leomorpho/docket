package vcs

import "context"

// Provider defines workflow-facing VCS operations.
type Provider interface {
	CreateWorktree(ctx context.Context, ticketID, branch, path string) error
	RemoveWorktree(ctx context.Context, path string) error
	GetAgentWorktreeDir(ctx context.Context, ticketID string) (string, error)
	GetRepoRoot(ctx context.Context) (string, error)
	CommitAll(ctx context.Context, worktreePath, message string) error
	MergeBranch(ctx context.Context, branch string) error
	DeleteBranch(ctx context.Context, branch string) error
}
