package semantic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

type DiffOp string

const (
	DiffAdd       DiffOp = "add"
	DiffChange    DiffOp = "change"
	DiffDelete    DiffOp = "delete"
	DiffUnchanged DiffOp = "unchanged"
)

type DiffEntry struct {
	Op       DiffOp
	Chunk    Chunk
	Previous ChunkMetadata
}

type FreshnessStatus string

const (
	FreshnessFresh            FreshnessStatus = "fresh"
	FreshnessStale            FreshnessStatus = "stale"
	FreshnessMissing          FreshnessStatus = "missing"
	FreshnessVersionMismatch  FreshnessStatus = "version_mismatch"
	FreshnessProviderMismatch FreshnessStatus = "provider_mismatch"
	FreshnessModelMismatch    FreshnessStatus = "model_mismatch"
)

type Freshness struct {
	Status FreshnessStatus
	Reason string
}

func EnumerateTickets(ctx context.Context, backend store.Backend) ([]*ticket.Ticket, error) {
	if localStore, ok := backend.(*local.Store); ok {
		return enumerateLocalTickets(ctx, localStore)
	}
	tickets, err := backend.ListTickets(ctx, store.Filter{IncludeArchived: false})
	if err != nil {
		return nil, err
	}
	return sortTickets(tickets), nil
}

func enumerateLocalTickets(ctx context.Context, backend *local.Store) ([]*ticket.Ticket, error) {
	paths, err := filepath.Glob(filepath.Join(backend.RepoRoot, ".docket", "tickets", "TKT-*.md"))
	if err != nil {
		return nil, err
	}

	tickets := make([]*ticket.Ticket, 0, len(paths))
	for _, path := range paths {
		id := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		tk, err := backend.GetTicket(ctx, id)
		if err != nil {
			return nil, err
		}
		if tk == nil || tk.State == "archived" {
			continue
		}
		tickets = append(tickets, tk)
	}
	return sortTickets(tickets), nil
}

func sortTickets(tickets []*ticket.Ticket) []*ticket.Ticket {
	sort.Slice(tickets, func(i, j int) bool {
		if tickets[i].Priority != tickets[j].Priority {
			return tickets[i].Priority < tickets[j].Priority
		}
		if !tickets[i].CreatedAt.Equal(tickets[j].CreatedAt) {
			return tickets[i].CreatedAt.Before(tickets[j].CreatedAt)
		}
		return tickets[i].ID < tickets[j].ID
	})
	return tickets
}

func DiffTickets(tickets []*ticket.Ticket, metadata *Metadata) []DiffEntry {
	current := map[string]Chunk{}
	var diffs []DiffEntry
	for _, t := range tickets {
		for _, chunk := range ChunkTicket(t) {
			current[chunk.ID] = chunk
			prev, ok := metadata.GetChunk(chunk.ID)
			switch {
			case !ok:
				diffs = append(diffs, DiffEntry{Op: DiffAdd, Chunk: chunk})
			case prev.Hash != chunk.Hash:
				diffs = append(diffs, DiffEntry{Op: DiffChange, Chunk: chunk, Previous: prev})
			default:
				diffs = append(diffs, DiffEntry{Op: DiffUnchanged, Chunk: chunk, Previous: prev})
			}
		}
	}

	for _, prev := range metadata.SortedChunks() {
		if _, ok := current[prev.ChunkID]; !ok {
			diffs = append(diffs, DiffEntry{
				Op:       DiffDelete,
				Chunk:    Chunk{ID: prev.ChunkID, TicketID: prev.TicketID, Type: prev.Type, Hash: prev.Hash},
				Previous: prev,
			})
		}
	}

	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].Chunk.ID != diffs[j].Chunk.ID {
			return diffs[i].Chunk.ID < diffs[j].Chunk.ID
		}
		return diffs[i].Op < diffs[j].Op
	})
	return diffs
}

type RebuildStats struct {
	Added     int
	Changed   int
	Deleted   int
	Unchanged int
}

func IncrementalRebuild(ctx context.Context, repoRoot string, provider Provider, providerCfg Config) (RebuildStats, error) {
	metadata, err := LoadMetadata(repoRoot)
	if err != nil {
		return RebuildStats{}, err
	}
	backend := local.New(repoRoot)
	tickets, err := EnumerateTickets(ctx, backend)
	if err != nil {
		return RebuildStats{}, err
	}
	diffs := DiffTickets(tickets, metadata)
	store, err := OpenVectorStore(repoRoot)
	if err != nil {
		return RebuildStats{}, err
	}

	stats := RebuildStats{}
	now := time.Now().UTC().Truncate(time.Second)
	for _, diff := range diffs {
		switch diff.Op {
		case DiffUnchanged:
			stats.Unchanged++
		case DiffDelete:
			if err := store.Delete(ctx, diff.Chunk.ID); err != nil {
				return stats, err
			}
			metadata.RemoveChunk(diff.Chunk.ID)
			stats.Deleted++
		case DiffAdd, DiffChange:
			resp, err := provider.Embed(ctx, EmbedRequest{
				Model: providerCfg.Model,
				Inputs: []Input{{
					ChunkID:  diff.Chunk.ID,
					TicketID: diff.Chunk.TicketID,
					Field:    string(diff.Chunk.Type),
					Text:     diff.Chunk.Text,
				}},
			})
			if err != nil {
				return stats, err
			}
			if len(resp.Results) != 1 {
				return stats, fmt.Errorf("expected one embedding result for %s, got %d", diff.Chunk.ID, len(resp.Results))
			}
			if err := store.Upsert(ctx, VectorDocument{
				ID:        diff.Chunk.ID,
				TicketID:  diff.Chunk.TicketID,
				Type:      diff.Chunk.Type,
				Hash:      diff.Chunk.Hash,
				Content:   diff.Chunk.Text,
				Embedding: float64To32(resp.Results[0].Vector),
			}); err != nil {
				return stats, err
			}
			metadata.UpsertChunk(ChunkMetadata{
				ChunkID:   diff.Chunk.ID,
				TicketID:  diff.Chunk.TicketID,
				Type:      diff.Chunk.Type,
				Hash:      diff.Chunk.Hash,
				UpdatedAt: now,
			})
			if diff.Op == DiffAdd {
				stats.Added++
			} else {
				stats.Changed++
			}
		}
	}

	metadata.Version = MetadataVersion
	metadata.Provider = provider.Name()
	metadata.Model = providerCfg.Model
	metadata.UpdatedAt = now
	return stats, SaveMetadata(repoRoot, metadata)
}

func FullRebuild(ctx context.Context, repoRoot string, provider Provider, providerCfg Config) (RebuildStats, error) {
	if err := os.RemoveAll(VectorDBPath(repoRoot)); err != nil {
		return RebuildStats{}, err
	}
	if err := SaveMetadata(repoRoot, NewMetadata()); err != nil {
		return RebuildStats{}, err
	}
	return IncrementalRebuild(ctx, repoRoot, provider, providerCfg)
}

func CheckFreshness(ctx context.Context, repoRoot string, providerCfg Config) (Freshness, error) {
	metadata, err := LoadMetadata(repoRoot)
	if err != nil {
		return Freshness{}, err
	}
	if len(metadata.Chunks) == 0 {
		return Freshness{Status: FreshnessMissing, Reason: "no semantic metadata present"}, nil
	}
	if metadata.Version != MetadataVersion {
		return Freshness{Status: FreshnessVersionMismatch, Reason: "metadata version mismatch"}, nil
	}
	if metadata.Provider != "" && metadata.Provider != providerCfg.Provider {
		return Freshness{Status: FreshnessProviderMismatch, Reason: "provider changed"}, nil
	}
	if metadata.Model != "" && metadata.Model != providerCfg.Model {
		return Freshness{Status: FreshnessModelMismatch, Reason: "model changed"}, nil
	}

	backend := local.New(repoRoot)
	tickets, err := EnumerateTickets(ctx, backend)
	if err != nil {
		return Freshness{}, err
	}
	for _, diff := range DiffTickets(tickets, metadata) {
		if diff.Op != DiffUnchanged {
			return Freshness{Status: FreshnessStale, Reason: "ticket chunks differ from metadata"}, nil
		}
	}
	return Freshness{Status: FreshnessFresh, Reason: "semantic index is current"}, nil
}

func float64To32(values []float64) []float32 {
	out := make([]float32, len(values))
	for i, value := range values {
		out[i] = float32(value)
	}
	return out
}
