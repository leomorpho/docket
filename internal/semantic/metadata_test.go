package semantic

import (
	"path/filepath"
	"testing"
	"time"
)

func TestMetadataPath(t *testing.T) {
	got := MetadataPath("/repo")
	want := filepath.Join("/repo", ".docket", "semantic", "metadata.json")
	if got != want {
		t.Fatalf("MetadataPath = %q, want %q", got, want)
	}
}

func TestLoadMetadataMissingFile(t *testing.T) {
	metadata, err := LoadMetadata(t.TempDir())
	if err != nil {
		t.Fatalf("LoadMetadata failed: %v", err)
	}
	if metadata.Version != MetadataVersion || len(metadata.Chunks) != 0 {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
}

func TestMetadataReadWriteRoundTrip(t *testing.T) {
	repo := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	metadata := NewMetadata()
	metadata.Provider = "uv"
	metadata.Model = "sentence-transformers/all-MiniLM-L6-v2"
	metadata.UpdatedAt = now
	metadata.UpsertChunk(ChunkMetadata{
		ChunkID:   "TKT-001:title",
		TicketID:  "TKT-001",
		Type:      ChunkTypeTitle,
		Hash:      "abc",
		UpdatedAt: now,
	})

	if err := SaveMetadata(repo, metadata); err != nil {
		t.Fatalf("SaveMetadata failed: %v", err)
	}

	loaded, err := LoadMetadata(repo)
	if err != nil {
		t.Fatalf("LoadMetadata failed: %v", err)
	}
	if loaded.Provider != metadata.Provider || loaded.Model != metadata.Model {
		t.Fatalf("unexpected metadata header: %+v", loaded)
	}
	chunk, ok := loaded.GetChunk("TKT-001:title")
	if !ok {
		t.Fatal("expected stored chunk")
	}
	if chunk.Hash != "abc" || chunk.Type != ChunkTypeTitle {
		t.Fatalf("unexpected chunk: %+v", chunk)
	}
}

func TestMetadataUpsertAndRemove(t *testing.T) {
	metadata := NewMetadata()
	metadata.UpsertChunk(ChunkMetadata{ChunkID: "c1", Hash: "a"})
	metadata.UpsertChunk(ChunkMetadata{ChunkID: "c1", Hash: "b"})
	chunk, ok := metadata.GetChunk("c1")
	if !ok || chunk.Hash != "b" {
		t.Fatalf("unexpected chunk after upsert: %+v", chunk)
	}
	metadata.RemoveChunk("c1")
	if _, ok := metadata.GetChunk("c1"); ok {
		t.Fatal("expected removed chunk to be absent")
	}
}

func TestMetadataSortedChunks(t *testing.T) {
	metadata := NewMetadata()
	metadata.UpsertChunk(ChunkMetadata{ChunkID: "b"})
	metadata.UpsertChunk(ChunkMetadata{ChunkID: "a"})
	sorted := metadata.SortedChunks()
	if len(sorted) != 2 || sorted[0].ChunkID != "a" || sorted[1].ChunkID != "b" {
		t.Fatalf("unexpected sorted chunks: %+v", sorted)
	}
}
