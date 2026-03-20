package selector

import (
	"context"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestServiceNextReturnsHighestPriorityRunnableTicket(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	for _, tc := range []struct {
		id       string
		priority int
		state    ticket.State
		blocked  []string
	}{
		{id: "TKT-101", priority: 2, state: ticket.State("todo")},
		{id: "TKT-102", priority: 1, state: ticket.State("todo")},
		{id: "TKT-103", priority: 1, state: ticket.State("todo"), blocked: []string{"TKT-101"}},
	} {
		if err := store.CreateTicket(context.Background(), &ticket.Ticket{
			ID:          tc.id,
			Seq:         100,
			Title:       tc.id,
			State:       tc.state,
			Priority:    tc.priority,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "desc",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
			BlockedBy:   tc.blocked,
		}); err != nil {
			t.Fatalf("create %s: %v", tc.id, err)
		}
	}

	selection, err := New(Dependencies{Store: store}).Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if !selection.Found || selection.TicketID != "TKT-102" {
		t.Fatalf("unexpected selection: %#v", selection)
	}
}

