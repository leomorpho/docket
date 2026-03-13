package cmd

import (
	"context"
	"testing"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestSelectNextTicket(t *testing.T) {
	tmpDir := t.TempDir()
	s := local.New(tmpDir)
	ctx := context.Background()

	// 1. Setup config
	cfg := ticket.DefaultConfig()
	// Add in-progress to backlog.next so transitions work in selectNextTicket
	backlog := cfg.States["backlog"]
	backlog.Next = append(backlog.Next, "in-progress")
	cfg.States["backlog"] = backlog
	
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// 2. Create some tickets
	// T1: priority 10, backlog
	// T2: priority 1, backlog (should be picked)
	// T3: priority 1, backlog, blocked (should be skipped)
	// T4: in-progress (should be skipped)

	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "T1", State: "backlog", Priority: 10})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-002", Seq: 2, Title: "T2", State: "backlog", Priority: 1})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-003", Seq: 3, Title: "T3", State: "backlog", Priority: 1, BlockedBy: []string{"TKT-001"}})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-004", Seq: 4, Title: "T4", State: "in-progress", Priority: 1})

	if err := s.SyncIndex(ctx); err != nil {
		t.Fatalf("SyncIndex failed: %v", err)
	}

	// 3. Select next
	got, err := selectNextTicket(ctx, s, cfg)
	if err != nil {
		t.Fatalf("selectNextTicket failed: %v", err)
	}

	if got == nil {
		t.Fatal("expected a ticket, got nil")
	}
	if got.ID != "TKT-002" {
		t.Errorf("expected TKT-002, got %s", got.ID)
	}
}
