package semantic

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const MetadataVersion = "v1"

type ChunkMetadata struct {
	ChunkID   string    `json:"chunk_id"`
	TicketID  string    `json:"ticket_id"`
	Type      ChunkType `json:"type"`
	Hash      string    `json:"hash"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Metadata struct {
	Version   string                   `json:"version"`
	Provider  string                   `json:"provider,omitempty"`
	Model     string                   `json:"model,omitempty"`
	UpdatedAt time.Time                `json:"updated_at,omitempty"`
	Chunks    map[string]ChunkMetadata `json:"chunks"`
}

func MetadataPath(repoRoot string) string {
	return filepath.Join(SemanticDir(repoRoot), "metadata.json")
}

func SemanticDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".docket", "semantic")
}

func VectorDBPath(repoRoot string) string {
	return filepath.Join(SemanticDir(repoRoot), "vector")
}

func LoadMetadata(repoRoot string) (*Metadata, error) {
	path := MetadataPath(repoRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewMetadata(), nil
		}
		return nil, err
	}

	var metadata Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}
	if metadata.Version == "" {
		metadata.Version = MetadataVersion
	}
	if metadata.Chunks == nil {
		metadata.Chunks = map[string]ChunkMetadata{}
	}
	return &metadata, nil
}

func SaveMetadata(repoRoot string, metadata *Metadata) error {
	if metadata == nil {
		metadata = NewMetadata()
	}
	if metadata.Version == "" {
		metadata.Version = MetadataVersion
	}
	if metadata.Chunks == nil {
		metadata.Chunks = map[string]ChunkMetadata{}
	}

	path := MetadataPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func NewMetadata() *Metadata {
	return &Metadata{
		Version: MetadataVersion,
		Chunks:  map[string]ChunkMetadata{},
	}
}

func (m *Metadata) UpsertChunk(chunk ChunkMetadata) {
	if m.Chunks == nil {
		m.Chunks = map[string]ChunkMetadata{}
	}
	m.Chunks[chunk.ChunkID] = chunk
}

func (m *Metadata) RemoveChunk(chunkID string) {
	if m.Chunks == nil {
		return
	}
	delete(m.Chunks, chunkID)
}

func (m *Metadata) GetChunk(chunkID string) (ChunkMetadata, bool) {
	if m.Chunks == nil {
		return ChunkMetadata{}, false
	}
	chunk, ok := m.Chunks[chunkID]
	return chunk, ok
}

func (m *Metadata) SortedChunks() []ChunkMetadata {
	out := make([]ChunkMetadata, 0, len(m.Chunks))
	for _, chunk := range m.Chunks {
		out = append(out, chunk)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ChunkID < out[j].ChunkID
	})
	return out
}
