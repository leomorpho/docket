package workflow

import (
	"context"

	"github.com/leomorpho/docket/internal/claim"
)

// VCSProvider defines the operations needed from a version control system.
type VCSProvider interface {
	CreateWorktree(ctx context.Context, ticketID, branch, path string) error
	RemoveWorktree(ctx context.Context, path string) error
	GetAgentWorktreeDir(ctx context.Context, ticketID string) (string, error)
	GetRepoRoot(ctx context.Context) (string, error)
	CommitAll(ctx context.Context, worktreePath, message string) error
	MergeBranch(ctx context.Context, branch string) error
	DeleteBranch(ctx context.Context, branch string) error
}

// ClaimManager defines the operations for ticket ownership.
type ClaimManager interface {
	Claim(ctx context.Context, ticketID, worktreePath, agentID string) error
	Release(ctx context.Context, ticketID string) error
	GetClaim(ctx context.Context, ticketID string) (*claim.ClaimMetadata, error)
}
