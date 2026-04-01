package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestQueueHealApplyUnblocksStartableLeaf(t *testing.T) {
	h := newFakeRepoHarness(t)
	s := local.New(h.repo)
	now := time.Now().UTC().Truncate(time.Second)

	for _, tk := range []*ticket.Ticket{
		{
			ID:          "TKT-201",
			Seq:         201,
			Title:       "Current blocker",
			State:       ticket.State("running"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "agent:test",
			Description: updateRunnableDescription(),
			AC:          updateRunnableAC(),
		},
		{
			ID:          "TKT-202",
			Seq:         202,
			Title:       "Blocked startable leaf",
			State:       ticket.State("ready"),
			Priority:    1,
			BlockedBy:   []string{"TKT-201"},
			CreatedAt:   now.Add(time.Minute),
			UpdatedAt:   now.Add(time.Minute),
			CreatedBy:   "agent:test",
			Description: updateRunnableDescription(),
			AC:          updateRunnableAC(),
		},
	} {
		if err := s.CreateTicket(context.Background(), tk); err != nil {
			t.Fatalf("seed %s failed: %v", tk.ID, err)
		}
	}

	before, err := h.run("list", "--state", "open", "--format", "context")
	if err != nil {
		t.Fatalf("list before failed: %v\n%s", err, before)
	}
	if !strings.Contains(before, "No workable tickets found.") {
		t.Fatalf("expected empty workable queue before heal, got:\n%s", before)
	}

	healOut, err := h.run("queue", "heal", "--apply")
	if err != nil {
		t.Fatalf("queue heal --apply failed: %v\n%s", err, healOut)
	}
	if !strings.Contains(healOut, "Applied queue heal") {
		t.Fatalf("expected applied heal message, got:\n%s", healOut)
	}

	after, err := h.run("list", "--state", "open", "--format", "context")
	if err != nil {
		t.Fatalf("list after failed: %v\n%s", err, after)
	}
	if strings.Contains(after, "No workable tickets found.") {
		t.Fatalf("expected workable queue after heal, got:\n%s", after)
	}
	if !strings.Contains(after, "TKT-202") {
		t.Fatalf("expected unblocked ticket in workable list, got:\n%s", after)
	}
}
