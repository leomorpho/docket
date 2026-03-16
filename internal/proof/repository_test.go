package proof

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFixturePNG(t *testing.T, path string) {
	t.Helper()
	// Minimal valid PNG signature + IHDR chunk bytes for MIME sniffing.
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

func TestProofRepositoryAdd_RequiredFieldsAndTimestampParsing(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	r := NewRepository(repoRoot)
	img := filepath.Join(repoRoot, "fixtures", "proof.png")
	writeFixturePNG(t, img)

	_, err := r.Add(context.Background(), AddInput{
		TicketID:   "TKT-240",
		SourcePath: img,
		ProofTitle: "",
		Note:       "",
		AddedAt:    "not-a-time",
		CapturedAt: "also-not-a-time",
		Actor:      "human:test",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	var ferr *FieldError
	if !errors.As(err, &ferr) {
		t.Fatalf("expected FieldError, got %T", err)
	}
	if ferr.Field == "" || ferr.ErrorCode == "" {
		t.Fatalf("expected structured error fields, got %+v", ferr)
	}
}

func TestProofRepositoryAdd_PersistsAndListsDeterministically(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	r := NewRepository(repoRoot)
	img := filepath.Join(repoRoot, "fixtures", "proof.png")
	writeFixturePNG(t, img)

	first, err := r.Add(context.Background(), AddInput{
		TicketID:   "TKT-240",
		SourcePath: img,
		ProofTitle: "Before",
		Note:       "Before screenshot",
		AddedAt:    "2026-03-16T17:10:00Z",
		Actor:      "human:test",
	})
	if err != nil {
		t.Fatalf("add first proof: %v", err)
	}
	second, err := r.Add(context.Background(), AddInput{
		TicketID:   "TKT-240",
		SourcePath: img,
		ProofTitle: "After",
		Note:       "After screenshot",
		AddedAt:    "2026-03-16T17:11:00Z",
		Actor:      "human:test",
	})
	if err != nil {
		t.Fatalf("add second proof: %v", err)
	}

	if first.File.SHA256 == "" || second.File.SHA256 == "" {
		t.Fatalf("expected checksum in metadata: first=%+v second=%+v", first.File, second.File)
	}
	if first.File.SHA256 != second.File.SHA256 {
		t.Fatalf("expected deterministic checksum for same bytes, got %q vs %q", first.File.SHA256, second.File.SHA256)
	}

	proofs, err := r.List(context.Background(), "TKT-240")
	if err != nil {
		t.Fatalf("list proofs: %v", err)
	}
	if len(proofs) != 2 {
		t.Fatalf("expected 2 proofs, got %d", len(proofs))
	}
	if proofs[0].AddedAt.After(proofs[1].AddedAt) {
		t.Fatalf("expected ascending deterministic ordering by added_at, got %s then %s", proofs[0].AddedAt, proofs[1].AddedAt)
	}
	if proofs[0].ID == "" || proofs[1].ID == "" {
		t.Fatalf("expected deterministic IDs, got %+v", proofs)
	}
	if proofs[0].File.Path == proofs[1].File.Path {
		t.Fatalf("expected unique persisted file path per proof record, got %q", proofs[0].File.Path)
	}

	for _, p := range proofs {
		if _, statErr := os.Stat(filepath.Join(repoRoot, p.File.Path)); statErr != nil {
			t.Fatalf("expected stored proof file %s: %v", p.File.Path, statErr)
		}
	}
}

func TestProofRepositoryAdd_RejectsUnsafeTicketIDAndMediaType(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	r := NewRepository(repoRoot)
	bad := filepath.Join(repoRoot, "fixtures", "proof.txt")
	if err := os.MkdirAll(filepath.Dir(bad), 0o755); err != nil {
		t.Fatalf("mkdir fixture dir: %v", err)
	}
	if err := os.WriteFile(bad, []byte("plain text"), 0o644); err != nil {
		t.Fatalf("write txt fixture: %v", err)
	}

	_, err := r.Add(context.Background(), AddInput{
		TicketID:   "../../etc/passwd",
		SourcePath: bad,
		ProofTitle: "Bad",
		Note:       "Bad",
		AddedAt:    time.Now().UTC().Format(time.RFC3339),
	})
	if err == nil {
		t.Fatal("expected unsafe ticket id rejection")
	}
	var ferr *FieldError
	if !errors.As(err, &ferr) {
		t.Fatalf("expected FieldError, got %T", err)
	}
	if ferr.Field != "ticket_id" {
		t.Fatalf("expected ticket_id field error, got %+v", ferr)
	}

	_, err = r.Add(context.Background(), AddInput{
		TicketID:   "TKT-240",
		SourcePath: bad,
		ProofTitle: "Bad",
		Note:       "Bad",
		AddedAt:    time.Now().UTC().Format(time.RFC3339),
	})
	if err == nil {
		t.Fatal("expected media type rejection")
	}
	if !errors.As(err, &ferr) {
		t.Fatalf("expected FieldError, got %T", err)
	}
	if ferr.Field != "mime_type" {
		t.Fatalf("expected mime_type field error, got %+v", ferr)
	}
}
