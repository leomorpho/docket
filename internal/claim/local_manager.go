package claim

import (
	"context"
)

type LocalClaimManager struct {
	repoRoot string
}

func NewLocalClaimManager(repoRoot string) *LocalClaimManager {
	return &LocalClaimManager{repoRoot: repoRoot}
}

func (m *LocalClaimManager) Claim(ctx context.Context, ticketID, worktreePath, agentID string) error {
	return Claim(m.repoRoot, ticketID, worktreePath, agentID)
}

func (m *LocalClaimManager) Release(ctx context.Context, ticketID string) error {
	return Release(m.repoRoot, ticketID)
}

func (m *LocalClaimManager) GetClaim(ctx context.Context, ticketID string) (*ClaimMetadata, error) {
	return GetClaim(m.repoRoot, ticketID)
}
