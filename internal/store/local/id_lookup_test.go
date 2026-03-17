package local

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/proof"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestStoreLookupAcceptsNonCanonicalTicketIDForms(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	s := New(repoRoot)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "Lookup normalization fixture",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
		Description: "desc",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	got, err := s.GetTicket(ctx, "[TKT-1]")
	if err != nil {
		t.Fatalf("get ticket by bracketed id: %v", err)
	}
	if got == nil || got.ID != "TKT-001" {
		t.Fatalf("expected TKT-001 via bracketed lookup, got %+v", got)
	}

	raw, err := s.GetRaw(ctx, "1")
	if err != nil {
		t.Fatalf("get raw by numeric id: %v", err)
	}
	if raw == "" {
		t.Fatalf("expected non-empty markdown via numeric id lookup")
	}
}

func TestStoreProofOpsAcceptNonCanonicalTicketIDForms(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	s := New(repoRoot)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-240",
		Seq:         240,
		Title:       "Proof lookup normalization fixture",
		State:       ticket.State("in-progress"),
		Priority:    2,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
		Description: "desc",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	imgRel := filepath.Join("fixtures", "proof.png")
	writeProofFixturePNG(t, filepath.Join(repoRoot, imgRel))

	rec, err := s.AddProof(ctx, proof.AddInput{
		TicketID:   "[TKT-240]",
		SourcePath: imgRel,
		ProofTitle: "Before",
		Note:       "baseline proof",
		AddedAt:    now.Format(time.RFC3339),
		Actor:      "agent:test",
	})
	if err != nil {
		t.Fatalf("add proof with bracketed id: %v", err)
	}

	listed, err := s.ListProofs(ctx, "240")
	if err != nil {
		t.Fatalf("list proofs with numeric id: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected one proof, got %d", len(listed))
	}

	if _, err := s.RemoveProof(ctx, "tkt-240", rec.ID); err != nil {
		t.Fatalf("remove proof with lowercase id: %v", err)
	}
}
