package proof

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProofBlobPolicy_ContentAddressedPathAndDedupe(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	r := NewRepository(repoRoot)

	imgA := filepath.Join(repoRoot, "fixtures", "a.png")
	imgB := filepath.Join(repoRoot, "fixtures", "b.png")
	writeFixturePNG(t, imgA)
	writeFixturePNG(t, imgB)

	first, err := r.Add(context.Background(), AddInput{
		TicketID:   "TKT-245",
		SourcePath: imgA,
		ProofTitle: "first",
		Note:       "first",
		AddedAt:    "2026-03-16T18:10:00Z",
	})
	if err != nil {
		t.Fatalf("add first: %v", err)
	}
	second, err := r.Add(context.Background(), AddInput{
		TicketID:   "TKT-245",
		SourcePath: imgB,
		ProofTitle: "second",
		Note:       "second",
		AddedAt:    "2026-03-16T18:11:00Z",
	})
	if err != nil {
		t.Fatalf("add second: %v", err)
	}

	if !strings.HasPrefix(first.File.Path, ".docket/proofs/by-hash/") {
		t.Fatalf("expected by-hash storage path, got %q", first.File.Path)
	}
	if first.File.Path != second.File.Path {
		t.Fatalf("expected deduped blob path for identical bytes, got %q and %q", first.File.Path, second.File.Path)
	}
	if !strings.Contains(first.File.Path, first.File.SHA256) {
		t.Fatalf("expected blob path to include sha256 %q, got %q", first.File.SHA256, first.File.Path)
	}

	blobDir := filepath.Join(repoRoot, ".docket", "proofs", "by-hash")
	entries, err := os.ReadDir(blobDir)
	if err != nil {
		t.Fatalf("read blob dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one deduped blob file, got %d", len(entries))
	}
}

func TestProofBlobPolicy_RegressesStableAddListCycles(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	r := NewRepository(repoRoot)
	img := filepath.Join(repoRoot, "fixtures", "a.png")
	writeFixturePNG(t, img)

	for i, ts := range []string{"2026-03-16T18:20:00Z", "2026-03-16T18:21:00Z", "2026-03-16T18:22:00Z"} {
		_, err := r.Add(context.Background(), AddInput{
			TicketID:   "TKT-245",
			SourcePath: img,
			ProofTitle: "cycle",
			Note:       "cycle",
			AddedAt:    ts,
		})
		if err != nil {
			t.Fatalf("add cycle %d: %v", i, err)
		}
		proofs, err := r.List(context.Background(), "TKT-245")
		if err != nil {
			t.Fatalf("list after cycle %d: %v", i, err)
		}
		if len(proofs) != i+1 {
			t.Fatalf("expected %d proofs after cycle %d, got %d", i+1, i, len(proofs))
		}
	}
}
