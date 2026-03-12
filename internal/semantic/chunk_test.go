package semantic

import (
	"testing"

	"github.com/leomorpho/docket/internal/ticket"
)

func TestChunkTicket(t *testing.T) {
	tk := &ticket.Ticket{
		ID:          "TKT-123",
		Title:       "Semantic indexing",
		Description: "Chunk description",
		AC: []ticket.AcceptanceCriterion{
			{Description: "AC one"},
			{Description: "AC two"},
		},
		Handoff: "handoff body",
	}

	chunks := ChunkTicket(tk)
	if len(chunks) != 5 {
		t.Fatalf("expected 5 chunks, got %d", len(chunks))
	}
	if chunks[0].ID != "TKT-123:title" || chunks[0].Type != ChunkTypeTitle {
		t.Fatalf("unexpected title chunk: %+v", chunks[0])
	}
	if chunks[2].ID != "TKT-123:ac:1" || chunks[3].ID != "TKT-123:ac:2" {
		t.Fatalf("unexpected AC chunk IDs: %+v %+v", chunks[2], chunks[3])
	}
	if chunks[4].ID != "TKT-123:handoff" || chunks[4].Type != ChunkTypeHandoff {
		t.Fatalf("unexpected handoff chunk: %+v", chunks[4])
	}
}

func TestChunkTicketSkipsEmptyFields(t *testing.T) {
	tk := &ticket.Ticket{
		ID:    "TKT-123",
		Title: "Only title",
		AC: []ticket.AcceptanceCriterion{
			{Description: ""},
		},
	}

	chunks := ChunkTicket(tk)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].ID != "TKT-123:title" {
		t.Fatalf("unexpected chunk: %+v", chunks[0])
	}
}

func TestChunkHashNormalization(t *testing.T) {
	a := chunkHash("hello\r\nworld\n")
	b := chunkHash("  hello\nworld  ")
	if a != b {
		t.Fatalf("expected normalized hashes to match: %s != %s", a, b)
	}
}

func TestChunkIDStability(t *testing.T) {
	tk := &ticket.Ticket{
		ID:          "TKT-123",
		Title:       "Title",
		Description: "Description",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
	}

	first := ChunkTicket(tk)
	second := ChunkTicket(tk)
	for i := range first {
		if first[i].ID != second[i].ID || first[i].Hash != second[i].Hash {
			t.Fatalf("chunk changed across rebuild: %+v != %+v", first[i], second[i])
		}
	}
}
