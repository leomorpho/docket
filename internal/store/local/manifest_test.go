package local

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/ticket"
)

func TestDetectTampering(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()

	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	tkt := &ticket.Ticket{
		ID: "TKT-001", Seq: 1, Title: "T1", State: "todo", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me",
		Description: "This is a long description so it passes validation.",
		AC: []ticket.AcceptanceCriterion{{Description: "A"}},
	}
	s.CreateTicket(ctx, tkt)

	// 1. No tampering
	changes, err := s.DetectTampering(ctx, "TKT-001")
	if err != nil {
		t.Fatalf("DetectTampering failed: %v", err)
	}
	if len(changes) > 0 {
		t.Errorf("expected no changes, got %v", changes)
	}

	// 2. Direct mutation
	p := filepath.Join(tmpDir, ".docket", "tickets", "TKT-001.md")
	raw, _ := os.ReadFile(p)
	edited := strings.Replace(string(raw), "priority: 1", "priority: 2", 1)
	os.WriteFile(p, []byte(edited), 0644)

	changes, err = s.DetectTampering(ctx, "TKT-001")
	if err != nil {
		t.Fatalf("DetectTampering failed after mutation: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected changes to be detected")
	}
	found := false
	for _, c := range changes {
		if c.Field == "priority" && c.Actual == "2" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("priority change not detected correctly: %v", changes)
	}
}

func TestReconcileTampering(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()

	ticket.SaveConfig(tmpDir, ticket.DefaultConfig())
	now := time.Now().UTC().Truncate(time.Second)
	
	// Create a ticket
	tkt := &ticket.Ticket{
		ID: "TKT-001", Seq: 1, Title: "T1", State: "todo", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me",
		Description: "This is a long description so it passes validation.",
		AC: []ticket.AcceptanceCriterion{{Description: "A"}},
	}
	s.CreateTicket(ctx, tkt)

	// 1. Mutate field to valid value
	p := filepath.Join(tmpDir, ".docket", "tickets", "TKT-001.md")
	raw, _ := os.ReadFile(p)
	edited := strings.Replace(string(raw), "priority: 1", "priority: 2", 1)
	os.WriteFile(p, []byte(edited), 0644)

	results, err := s.ReconcileTampering(ctx)
	if err != nil {
		t.Fatalf("ReconcileTampering failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Accepted {
		t.Errorf("expected direct edit to be accepted")
	}

	// 2. Mutate to invalid state
	raw, _ = os.ReadFile(p)
	edited = strings.Replace(string(raw), "state: todo", "state: junk", 1)
	os.WriteFile(p, []byte(edited), 0644)

	results, err = s.ReconcileTampering(ctx)
	if err != nil {
		t.Fatalf("ReconcileTampering failed: %v", err)
	}
	if results[0].Accepted {
		t.Errorf("expected invalid state edit to be rejected")
	}
	if !results[0].Reverted {
		t.Errorf("expected invalid state edit to be reverted")
	}
}
