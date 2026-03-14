package workflow

import (
	"context"

	"github.com/leomorpho/docket/internal/claim"
)

// ClaimManager defines the operations for ticket ownership.
type ClaimManager interface {
	Claim(ctx context.Context, ticketID, worktreePath, agentID string) error
	Release(ctx context.Context, ticketID string) error
	GetClaim(ctx context.Context, ticketID string) (*claim.ClaimMetadata, error)
}
