package semantic

import (
	"context"
	"fmt"
	"strings"

	chromem "github.com/philippgille/chromem-go"
)

const vectorCollectionName = "semantic_chunks"

type VectorDocument struct {
	ID        string
	TicketID  string
	Type      ChunkType
	Hash      string
	Content   string
	Embedding []float32
}

type QueryResult struct {
	ID         string
	TicketID   string
	Type       ChunkType
	Hash       string
	Content    string
	Similarity float32
}

type VectorStore struct {
	db         *chromem.DB
	collection *chromem.Collection
}

func OpenVectorStore(repoRoot string) (*VectorStore, error) {
	db, err := chromem.NewPersistentDB(VectorDBPath(repoRoot), false)
	if err != nil {
		return nil, fmt.Errorf("open vector db: %w", err)
	}

	collection, err := db.GetOrCreateCollection(vectorCollectionName, map[string]string{
		"scope": "semantic",
		"kind":  "ticket_chunks",
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("open vector collection: %w", err)
	}

	return &VectorStore{db: db, collection: collection}, nil
}

func (s *VectorStore) Upsert(ctx context.Context, doc VectorDocument) error {
	if s == nil || s.collection == nil {
		return fmt.Errorf("vector store is not initialized")
	}
	if doc.ID == "" {
		return fmt.Errorf("vector document ID is empty")
	}

	_ = s.collection.Delete(ctx, nil, nil, doc.ID)
	return s.collection.AddDocument(ctx, chromem.Document{
		ID:        doc.ID,
		Metadata:  map[string]string{"ticket_id": doc.TicketID, "chunk_type": string(doc.Type), "hash": doc.Hash},
		Embedding: doc.Embedding,
		Content:   doc.Content,
	})
}

func (s *VectorStore) Delete(ctx context.Context, id string) error {
	if s == nil || s.collection == nil {
		return fmt.Errorf("vector store is not initialized")
	}
	if id == "" {
		return fmt.Errorf("vector document ID is empty")
	}
	return s.collection.Delete(ctx, nil, nil, id)
}

func (s *VectorStore) Query(ctx context.Context, embedding []float32, limit int) ([]QueryResult, error) {
	if s == nil || s.collection == nil {
		return nil, fmt.Errorf("vector store is not initialized")
	}
	results, err := s.collection.QueryEmbedding(ctx, embedding, limit, nil, nil)
	if err != nil {
		if strings.Contains(err.Error(), "nResults must be <=") {
			results, err = s.collection.QueryEmbedding(ctx, embedding, 1, nil, nil)
		}
	}
	if err != nil {
		return nil, err
	}

	out := make([]QueryResult, 0, len(results))
	for _, result := range results {
		out = append(out, QueryResult{
			ID:         result.ID,
			TicketID:   result.Metadata["ticket_id"],
			Type:       ChunkType(result.Metadata["chunk_type"]),
			Hash:       result.Metadata["hash"],
			Content:    result.Content,
			Similarity: result.Similarity,
		})
	}
	return out, nil
}
