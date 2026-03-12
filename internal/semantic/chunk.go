package semantic

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/leomorpho/docket/internal/ticket"
)

type ChunkType string

const (
	ChunkTypeTitle       ChunkType = "title"
	ChunkTypeDescription ChunkType = "description"
	ChunkTypeAC          ChunkType = "ac"
	ChunkTypeHandoff     ChunkType = "handoff"
)

// Chunk is the canonical semantic indexing unit produced from a ticket.
// Only title, description, individual AC descriptions, and handoff text are chunked.
type Chunk struct {
	ID       string
	TicketID string
	Type     ChunkType
	Index    int
	Text     string
	Hash     string
}

func ChunkTicket(t *ticket.Ticket) []Chunk {
	if t == nil {
		return nil
	}

	var chunks []Chunk
	appendChunk := func(chunkType ChunkType, index int, text string) {
		if normalizeChunkText(text) == "" {
			return
		}
		chunks = append(chunks, Chunk{
			ID:       chunkID(t.ID, chunkType, index),
			TicketID: t.ID,
			Type:     chunkType,
			Index:    index,
			Text:     text,
			Hash:     chunkHash(text),
		})
	}

	appendChunk(ChunkTypeTitle, 0, t.Title)
	appendChunk(ChunkTypeDescription, 0, t.Description)
	for i, ac := range t.AC {
		appendChunk(ChunkTypeAC, i+1, ac.Description)
	}
	appendChunk(ChunkTypeHandoff, 0, t.Handoff)

	return chunks
}

func chunkID(ticketID string, chunkType ChunkType, index int) string {
	switch chunkType {
	case ChunkTypeAC:
		return fmt.Sprintf("%s:%s:%d", ticketID, chunkType, index)
	default:
		return fmt.Sprintf("%s:%s", ticketID, chunkType)
	}
}

func chunkHash(text string) string {
	sum := sha256.Sum256([]byte(normalizeChunkText(text)))
	return hex.EncodeToString(sum[:])
}

func normalizeChunkText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimSpace(text)
}
