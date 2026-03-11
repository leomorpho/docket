package semantic

import (
	"context"
	"testing"
	"time"

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
)

type fakeProvider struct {
	name    string
	calls   int
	vectors map[string][]float64
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Status(context.Context) (Status, error) {
	return Status{Provider: f.name, Available: true}, nil
}
func (f *fakeProvider) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	f.calls++
	results := make([]EmbedResult, 0, len(req.Inputs))
	for _, input := range req.Inputs {
		vector, ok := f.vectors[input.ChunkID]
		if !ok {
			vector = []float64{1, 0}
		}
		results = append(results, EmbedResult{ChunkID: input.ChunkID, Vector: vector})
	}
	return EmbedResponse{Model: req.Model, Dimension: len(results[0].Vector), Results: results}, nil
}

func seedTickets(t *testing.T, repo string, tickets ...*ticket.Ticket) {
	t.Helper()
	store := local.New(repo)
	if err := ticket.SaveConfig(repo, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	for _, tk := range tickets {
		if err := store.CreateTicket(context.Background(), tk); err != nil {
			t.Fatalf("CreateTicket failed: %v", err)
		}
	}
}

func TestEnumerateTickets(t *testing.T) {
	repo := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	seedTickets(t, repo,
		&ticket.Ticket{ID: "TKT-002", Seq: 2, Title: "done", State: ticket.State("done"), Priority: 2, CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute), CreatedBy: "me"},
		&ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "todo", State: ticket.State("todo"), Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me"},
		&ticket.Ticket{ID: "TKT-003", Seq: 3, Title: "archived", State: ticket.State("archived"), Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me"},
	)
	got, err := EnumerateTickets(context.Background(), local.New(repo))
	if err != nil {
		t.Fatalf("EnumerateTickets failed: %v", err)
	}
	if len(got) != 2 || got[0].ID != "TKT-001" || got[1].ID != "TKT-002" {
		t.Fatalf("unexpected tickets: %#v", got)
	}
}

func TestDiffTickets(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	tickets := []*ticket.Ticket{{
		ID:          "TKT-001",
		Title:       "Title",
		Description: "Desc",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
		Handoff:     "Handoff",
		CreatedAt:   now,
		UpdatedAt:   now,
	}}
	metadata := NewMetadata()
	chunks := ChunkTicket(tickets[0])
	metadata.UpsertChunk(ChunkMetadata{ChunkID: chunks[0].ID, TicketID: "TKT-001", Type: chunks[0].Type, Hash: chunks[0].Hash, UpdatedAt: now})
	metadata.UpsertChunk(ChunkMetadata{ChunkID: "TKT-999:title", TicketID: "TKT-999", Type: ChunkTypeTitle, Hash: "old", UpdatedAt: now})

	diffs := DiffTickets(tickets, metadata)
	var adds, changes, deletes, unchanged int
	for _, diff := range diffs {
		switch diff.Op {
		case DiffAdd:
			adds++
		case DiffChange:
			changes++
		case DiffDelete:
			deletes++
		case DiffUnchanged:
			unchanged++
		}
	}
	if adds == 0 || deletes != 1 || unchanged != 1 || changes != 0 {
		t.Fatalf("unexpected diff counts: adds=%d changes=%d deletes=%d unchanged=%d", adds, changes, deletes, unchanged)
	}
}

func TestIncrementalRebuildAndFreshness(t *testing.T) {
	repo := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	seedTickets(t, repo, &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "Semantic",
		Description: "Description",
		AC:          []ticket.AcceptanceCriterion{{Description: "A1"}},
		Handoff:     "Handoff",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
	})

	cfg := Config{Provider: "fake", Model: "model"}
	provider := &fakeProvider{name: "fake", vectors: map[string][]float64{}}
	stats, err := IncrementalRebuild(context.Background(), repo, provider, cfg)
	if err != nil {
		t.Fatalf("IncrementalRebuild failed: %v", err)
	}
	if stats.Added == 0 || provider.calls == 0 {
		t.Fatalf("expected add work and provider calls: %+v calls=%d", stats, provider.calls)
	}

	fresh, err := CheckFreshness(context.Background(), repo, cfg)
	if err != nil {
		t.Fatalf("CheckFreshness failed: %v", err)
	}
	if fresh.Status != FreshnessFresh {
		t.Fatalf("expected fresh, got %+v", fresh)
	}

	provider.calls = 0
	stats, err = IncrementalRebuild(context.Background(), repo, provider, cfg)
	if err != nil {
		t.Fatalf("IncrementalRebuild second run failed: %v", err)
	}
	if stats.Added != 0 || stats.Changed != 0 || stats.Deleted != 0 || provider.calls != 0 {
		t.Fatalf("expected no-op rebuild, got %+v calls=%d", stats, provider.calls)
	}

	store := local.New(repo)
	tk, err := store.GetTicket(context.Background(), "TKT-001")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	tk.Description = "Changed"
	tk.UpdatedAt = now.Add(time.Minute)
	if err := store.UpdateTicket(context.Background(), tk); err != nil {
		t.Fatalf("UpdateTicket failed: %v", err)
	}

	fresh, err = CheckFreshness(context.Background(), repo, cfg)
	if err != nil {
		t.Fatalf("CheckFreshness stale failed: %v", err)
	}
	if fresh.Status != FreshnessStale {
		t.Fatalf("expected stale, got %+v", fresh)
	}
}

func TestFullRebuildAndVersionMismatch(t *testing.T) {
	repo := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	seedTickets(t, repo, &ticket.Ticket{
		ID:        "TKT-001",
		Seq:       1,
		Title:     "Semantic",
		State:     ticket.State("todo"),
		Priority:  1,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "me",
	})

	cfg := Config{Provider: "fake", Model: "model"}
	provider := &fakeProvider{name: "fake", vectors: map[string][]float64{}}
	if _, err := FullRebuild(context.Background(), repo, provider, cfg); err != nil {
		t.Fatalf("FullRebuild failed: %v", err)
	}

	metadata, err := LoadMetadata(repo)
	if err != nil {
		t.Fatalf("LoadMetadata failed: %v", err)
	}
	metadata.Version = "old"
	if err := SaveMetadata(repo, metadata); err != nil {
		t.Fatalf("SaveMetadata failed: %v", err)
	}

	fresh, err := CheckFreshness(context.Background(), repo, cfg)
	if err != nil {
		t.Fatalf("CheckFreshness failed: %v", err)
	}
	if fresh.Status != FreshnessVersionMismatch {
		t.Fatalf("expected version mismatch, got %+v", fresh)
	}
}
