package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/proof"
	"github.com/leomorpho/docket/internal/ticket"
)

func writeProofFixturePNG(t *testing.T, path string) {
	t.Helper()
	data := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde,
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture dir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write png fixture: %v", err)
	}
}

func TestStoreProofLifecycle_DoesNotMutateTicketMarkdown(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	s := New(repoRoot)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	tkt := &ticket.Ticket{
		ID:          "TKT-240",
		Seq:         240,
		Title:       "Proof metadata storage",
		State:       ticket.State("in-progress"),
		Priority:    2,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "desc",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
	}
	if err := s.CreateTicket(ctx, tkt); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	beforeRaw, err := s.GetRaw(ctx, "TKT-240")
	if err != nil {
		t.Fatalf("get raw before: %v", err)
	}

	img := filepath.Join(repoRoot, "fixtures", "proof.png")
	writeProofFixturePNG(t, img)

	rec, err := s.AddProof(ctx, proof.AddInput{
		TicketID:   "TKT-240",
		SourcePath: img,
		ProofTitle: "CLI output before",
		Note:       "Capture baseline behavior",
		AddedAt:    now.Format(time.RFC3339),
		Actor:      "human:test",
	})
	if err != nil {
		t.Fatalf("add proof: %v", err)
	}
	if rec.ID == "" || rec.File.Path == "" {
		t.Fatalf("expected proof metadata to be populated: %+v", rec)
	}

	afterRaw, err := s.GetRaw(ctx, "TKT-240")
	if err != nil {
		t.Fatalf("get raw after: %v", err)
	}
	if beforeRaw != afterRaw {
		t.Fatalf("expected ticket markdown unchanged by proof storage")
	}

	proofs, err := s.ListProofs(ctx, "TKT-240")
	if err != nil {
		t.Fatalf("list proofs: %v", err)
	}
	if len(proofs) != 1 {
		t.Fatalf("expected one stored proof, got %d", len(proofs))
	}
	if proofs[0].ProofTitle != "CLI output before" || proofs[0].Note != "Capture baseline behavior" {
		t.Fatalf("unexpected stored metadata: %+v", proofs[0])
	}
}

func TestStoreAddProof_RequiresExistingTicket(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	s := New(repoRoot)
	ctx := context.Background()
	img := filepath.Join(repoRoot, "fixtures", "proof.png")
	writeProofFixturePNG(t, img)

	_, err := s.AddProof(ctx, proof.AddInput{
		TicketID:   "TKT-999",
		SourcePath: img,
		ProofTitle: "Missing",
		Note:       "Missing",
		AddedAt:    time.Now().UTC().Format(time.RFC3339),
	})
	if err == nil {
		t.Fatal("expected missing ticket error")
	}
}
