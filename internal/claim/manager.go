package claim

import "context"

// Manager defines ticket claim ownership operations.
type Manager interface {
	Claim(ctx context.Context, ticketID, worktreePath, agentID string) error
	Release(ctx context.Context, ticketID string) error
	GetClaim(ctx context.Context, ticketID string) (*ClaimMetadata, error)
}
