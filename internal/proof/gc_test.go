package proof

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestProofGC_RemoveSemanticsAndSharedBlobSafety(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	r := NewRepository(repoRoot)
	imgRel := filepath.Join("fixtures", "proof.png")
	imgAbs := filepath.Join(repoRoot, imgRel)
	writeFixturePNG(t, imgAbs)

	p1, err := r.Add(context.Background(), AddInput{
		TicketID:   "TKT-247",
		SourcePath: imgRel,
		ProofTitle: "A",
		Note:       "A",
		AddedAt:    "2026-03-16T19:30:00Z",
	})
	if err != nil {
		t.Fatalf("add first proof: %v", err)
	}
	p2, err := r.Add(context.Background(), AddInput{
		TicketID:   "TKT-248",
		SourcePath: imgRel,
		ProofTitle: "B",
		Note:       "B",
		AddedAt:    "2026-03-16T19:31:00Z",
	})
	if err != nil {
		t.Fatalf("add second proof: %v", err)
	}
	if p1.File.Path != p2.File.Path {
		t.Fatalf("expected shared blob path, got %q and %q", p1.File.Path, p2.File.Path)
	}

	if _, err := r.Remove(context.Background(), "TKT-247", p1.ID); err != nil {
		t.Fatalf("remove first proof metadata: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(repoRoot, p1.File.Path)); statErr != nil {
		t.Fatalf("expected shared blob to remain after single remove: %v", statErr)
	}

	summary, err := r.GC(context.Background())
	if err != nil {
		t.Fatalf("gc while blob still referenced: %v", err)
	}
	if summary.Removed != 0 {
		t.Fatalf("expected removed=0 while referenced, got %+v", summary)
	}

	if _, err := r.Remove(context.Background(), "TKT-248", p2.ID); err != nil {
		t.Fatalf("remove second proof metadata: %v", err)
	}
	summary, err = r.GC(context.Background())
	if err != nil {
		t.Fatalf("gc after unreference: %v", err)
	}
	if summary.Removed != 1 {
		t.Fatalf("expected one blob removed, got %+v", summary)
	}
	if _, statErr := os.Stat(filepath.Join(repoRoot, p1.File.Path)); !os.IsNotExist(statErr) {
		t.Fatalf("expected blob to be removed, stat err=%v", statErr)
	}
}
