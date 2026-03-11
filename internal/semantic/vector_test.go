package semantic

import (
	"context"
	"testing"
)

func TestSemanticPaths(t *testing.T) {
	repo := "/repo"
	if got := SemanticDir(repo); got != "/repo/.docket/semantic" {
		t.Fatalf("SemanticDir = %q", got)
	}
	if got := VectorDBPath(repo); got != "/repo/.docket/semantic/vector" {
		t.Fatalf("VectorDBPath = %q", got)
	}
	if got := MetadataPath(repo); got != "/repo/.docket/semantic/metadata.json" {
		t.Fatalf("MetadataPath = %q", got)
	}
}

func TestVectorStoreOpenUpsertQueryDelete(t *testing.T) {
	repo := t.TempDir()
	store, err := OpenVectorStore(repo)
	if err != nil {
		t.Fatalf("OpenVectorStore failed: %v", err)
	}

	ctx := context.Background()
	if err := store.Upsert(ctx, VectorDocument{
		ID:        "chunk-1",
		TicketID:  "TKT-001",
		Type:      ChunkTypeTitle,
		Hash:      "hash-1",
		Content:   "first",
		Embedding: []float32{1, 0},
	}); err != nil {
		t.Fatalf("Upsert chunk-1 failed: %v", err)
	}
	if err := store.Upsert(ctx, VectorDocument{
		ID:        "chunk-2",
		TicketID:  "TKT-002",
		Type:      ChunkTypeDescription,
		Hash:      "hash-2",
		Content:   "second",
		Embedding: []float32{0, 1},
	}); err != nil {
		t.Fatalf("Upsert chunk-2 failed: %v", err)
	}

	results, err := store.Query(ctx, []float32{1, 0}, 2)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "chunk-1" || results[0].TicketID != "TKT-001" || results[0].Type != ChunkTypeTitle {
		t.Fatalf("unexpected first result: %+v", results[0])
	}

	if err := store.Upsert(ctx, VectorDocument{
		ID:        "chunk-1",
		TicketID:  "TKT-001",
		Type:      ChunkTypeTitle,
		Hash:      "hash-1b",
		Content:   "first-updated",
		Embedding: []float32{1, 0},
	}); err != nil {
		t.Fatalf("Upsert replace failed: %v", err)
	}
	results, err = store.Query(ctx, []float32{1, 0}, 2)
	if err != nil {
		t.Fatalf("Query after replace failed: %v", err)
	}
	if results[0].Hash != "hash-1b" || results[0].Content != "first-updated" {
		t.Fatalf("expected replaced result, got %+v", results[0])
	}

	if err := store.Delete(ctx, "chunk-2"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	results, err = store.Query(ctx, []float32{0, 1}, 1)
	if err != nil {
		t.Fatalf("Query after delete failed: %v", err)
	}
	if len(results) != 1 || results[0].ID != "chunk-1" {
		t.Fatalf("unexpected results after delete: %+v", results)
	}

	if err := store.Delete(ctx, "does-not-exist"); err != nil {
		t.Fatalf("Delete missing ID should be safe, got %v", err)
	}
}

func TestVectorStoreReopenRoundTripsMetadata(t *testing.T) {
	repo := t.TempDir()
	ctx := context.Background()

	store, err := OpenVectorStore(repo)
	if err != nil {
		t.Fatalf("OpenVectorStore failed: %v", err)
	}
	if err := store.Upsert(ctx, VectorDocument{
		ID:        "chunk-1",
		TicketID:  "TKT-010",
		Type:      ChunkTypeAC,
		Hash:      "hash-10",
		Content:   "acceptance criteria",
		Embedding: []float32{0.4, 0.6},
	}); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	reopened, err := OpenVectorStore(repo)
	if err != nil {
		t.Fatalf("OpenVectorStore reopen failed: %v", err)
	}
	results, err := reopened.Query(ctx, []float32{0.4, 0.6}, 1)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].TicketID != "TKT-010" || results[0].Type != ChunkTypeAC || results[0].Hash != "hash-10" {
		t.Fatalf("unexpected round-tripped metadata: %+v", results[0])
	}
}
