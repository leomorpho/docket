package semantic

import (
	"context"
	"testing"

	"github.com/leoaudibert/docket/internal/ticket"
)

type vectorTestProvider struct {
	response EmbedResponse
}

func (p *vectorTestProvider) Name() string { return "test" }
func (p *vectorTestProvider) Status(context.Context) (Status, error) {
	return Status{Provider: "test", Available: true}, nil
}
func (p *vectorTestProvider) Embed(context.Context, EmbedRequest) (EmbedResponse, error) {
	return p.response, nil
}

func TestSimpleLexicalScorerUsesFieldWeights(t *testing.T) {
	source := &ticket.Ticket{
		ID:          "TKT-001",
		Title:       "semantic search index",
		Description: "build local vector index",
		AC:          []ticket.AcceptanceCriterion{{Description: "return related tickets"}},
		Handoff:     "semantic mode enabled",
	}
	candidates := []*ticket.Ticket{
		{ID: "TKT-002", Title: "semantic search ranking"},
		{ID: "TKT-003", Description: "build local vector index"},
		{ID: "TKT-004", Handoff: "semantic mode enabled"},
	}

	scores := SimpleLexicalScorer{}.ScoreRelated(source, candidates, DefaultFieldWeights())
	if len(scores) != 3 {
		t.Fatalf("expected 3 lexical scores, got %d", len(scores))
	}
	if scores[0].TicketID != "TKT-002" {
		t.Fatalf("expected title match first, got %#v", scores)
	}
	if scores[0].Fields[ChunkTypeTitle] <= scores[1].Fields[ChunkTypeDescription] {
		t.Fatalf("expected title weight to dominate: %#v", scores)
	}
}

func TestCombineScoresKeepsSingleSourceTickets(t *testing.T) {
	cfg := Config{LexicalWeight: 0.4, VectorWeight: 0.6}
	combined := CombineScores(cfg,
		[]LexicalScore{{TicketID: "TKT-002", Score: 0.8}},
		[]VectorScore{{TicketID: "TKT-003", Score: 0.9}},
	)
	if len(combined) != 2 {
		t.Fatalf("expected two combined scores, got %d", len(combined))
	}
	if combined[0].TicketID != "TKT-003" || combined[1].TicketID != "TKT-002" {
		t.Fatalf("unexpected combined order: %#v", combined)
	}
}

func TestCombineScoresHybridOrdering(t *testing.T) {
	cfg := Config{LexicalWeight: 0.35, VectorWeight: 0.65}
	combined := CombineScores(cfg,
		[]LexicalScore{
			{TicketID: "TKT-002", Score: 0.9},
			{TicketID: "TKT-003", Score: 0.2},
		},
		[]VectorScore{
			{TicketID: "TKT-003", Score: 1.0},
			{TicketID: "TKT-002", Score: 0.1},
		},
	)
	if len(combined) != 2 || combined[0].TicketID != "TKT-003" {
		t.Fatalf("expected vector-heavy ticket first, got %#v", combined)
	}
}

func TestBuildGraphInputsAndBoosts(t *testing.T) {
	source := &ticket.Ticket{
		ID:            "TKT-001",
		BlockedBy:     []string{"TKT-002"},
		LinkedCommits: []string{"abc123"},
	}
	candidates := []*ticket.Ticket{
		{ID: "TKT-002"},
		{ID: "TKT-003", LinkedCommits: []string{"abc123"}},
	}

	inputs := BuildGraphInputs(source, candidates)
	if len(inputs.Edges) != 2 {
		t.Fatalf("expected 2 graph edges, got %#v", inputs)
	}
	boosts := CalculateGraphBoosts(inputs)
	if boosts["TKT-002"] <= boosts["TKT-003"] {
		t.Fatalf("expected dependency boost to exceed linked-commit boost: %#v", boosts)
	}
}

func TestApplyGraphBoostsChangesOrder(t *testing.T) {
	scores := []CombinedScore{
		{TicketID: "TKT-002", Score: 0.55},
		{TicketID: "TKT-003", Score: 0.6},
	}
	boosted := ApplyGraphBoosts(scores, map[string]float64{
		"TKT-002": 0.1,
	})
	if boosted[0].TicketID != "TKT-002" {
		t.Fatalf("expected graph boost to change ordering, got %#v", boosted)
	}
}

func TestVectorScorerAggregatesFieldScores(t *testing.T) {
	repo := t.TempDir()
	store, err := OpenVectorStore(repo)
	if err != nil {
		t.Fatalf("OpenVectorStore failed: %v", err)
	}
	if err := store.Upsert(context.Background(), VectorDocument{
		ID:        "TKT-002:title",
		TicketID:  "TKT-002",
		Type:      ChunkTypeTitle,
		Hash:      "a",
		Content:   "semantic search ranking",
		Embedding: []float32{0.9, 0.1},
	}); err != nil {
		t.Fatalf("Upsert title failed: %v", err)
	}
	if err := store.Upsert(context.Background(), VectorDocument{
		ID:        "TKT-003:description",
		TicketID:  "TKT-003",
		Type:      ChunkTypeDescription,
		Hash:      "b",
		Content:   "vector index",
		Embedding: []float32{0.8, 0.2},
	}); err != nil {
		t.Fatalf("Upsert description failed: %v", err)
	}

	source := &ticket.Ticket{ID: "TKT-001", Title: "semantic search"}
	scorer := VectorScorer{
		Provider: &vectorTestProvider{response: EmbedResponse{
			Model: "test",
			Results: []EmbedResult{{
				ChunkID: "TKT-001:title",
				Vector:  []float64{0.9, 0.1},
			}},
		}},
		Store: store,
	}

	scores, err := scorer.ScoreRelated(context.Background(), source, Config{
		Model:       "test",
		TitleWeight: 3.0,
	}, 5)
	if err != nil {
		t.Fatalf("ScoreRelated failed: %v", err)
	}
	if len(scores) == 0 || scores[0].TicketID != "TKT-002" {
		t.Fatalf("unexpected vector scores: %#v", scores)
	}
}
