package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func enforceLeafExecutionBlockers(ctx context.Context, s *local.Store, blockerIDs []string) error {
	for _, blockerID := range blockerIDs {
		id := strings.TrimSpace(blockerID)
		if id == "" {
			continue
		}
		blocker, err := s.GetTicket(ctx, id)
		if err != nil {
			return fmt.Errorf("loading blocker %s: %w", id, err)
		}
		if blocker == nil {
			continue
		}
		isLeaf, err := s.IsLeafTicket(ctx, blocker.ID)
		if err != nil {
			return fmt.Errorf("checking blocker %s children: %w", id, err)
		}
		if ticket.IsCoordinationTicket(blocker) || !isLeaf {
			return fmt.Errorf("execution blocker %s must be a leaf ticket and cannot be a coordination ticket", id)
		}
	}
	return nil
}
