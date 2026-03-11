package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/leoaudibert/docket/internal/semantic"
	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
)

type semanticE2EProvider struct{}

func (p *semanticE2EProvider) Name() string { return "fake" }
func (p *semanticE2EProvider) Status(context.Context) (semantic.Status, error) {
	return semantic.Status{Provider: "fake", Model: "fake-model", Available: true}, nil
}
func (p *semanticE2EProvider) Embed(ctx context.Context, req semantic.EmbedRequest) (semantic.EmbedResponse, error) {
	results := make([]semantic.EmbedResult, 0, len(req.Inputs))
	for _, input := range req.Inputs {
		vector := []float64{0, 1}
		if strings.Contains(strings.ToLower(input.Text), "semantic") || strings.Contains(strings.ToLower(input.Text), "vector") {
			vector = []float64{1, 0}
		}
		results = append(results, semantic.EmbedResult{
			ChunkID: input.ChunkID,
			Vector:  vector,
		})
	}
	return semantic.EmbedResponse{Model: req.Model, Dimension: 2, Results: results}, nil
}

func TestRelatedCmdLexicalOnly(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	relatedSemantic = "off"
	relatedLimit = 5

	seedRelatedTickets(t, tmpDir,
		&ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "semantic search", Description: "local vector index", State: ticket.StateTodo, Priority: 1},
		&ticket.Ticket{ID: "TKT-002", Seq: 2, Title: "semantic ranking", Description: "search", State: ticket.StateTodo, Priority: 1},
		&ticket.Ticket{ID: "TKT-003", Seq: 3, Title: "release notes", State: ticket.StateTodo, Priority: 1},
	)

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"related", "TKT-001", "--semantic", "off", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var got relatedView
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatalf("json decode failed: %v", err)
	}
	if got.SemanticUsed {
		t.Fatalf("expected lexical-only execution, got %#v", got)
	}
	if len(got.Results) == 0 || got.Results[0].TicketID != "TKT-002" {
		t.Fatalf("unexpected related results: %#v", got.Results)
	}
}

func TestRelatedCmdRejectsInvalidSemanticMode(t *testing.T) {
	repo = t.TempDir()
	format = "human"
	relatedSemantic = "weird"
	rootCmd.SetArgs([]string{"related", "TKT-001", "--semantic", "weird"})
	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid semantic mode") {
		t.Fatalf("expected invalid semantic mode error, got %v", err)
	}
}

func TestRelatedCmdAutoFallbackWarns(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	relatedSemantic = "auto"
	relatedLimit = 5

	cfg := ticket.DefaultConfig()
	cfg.Semantic.Enabled = true
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	seedRelatedTicketsWithoutConfig(t, tmpDir,
		&ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "semantic search", State: ticket.StateTodo, Priority: 1},
		&ticket.Ticket{ID: "TKT-002", Seq: 2, Title: "semantic ranking", State: ticket.StateTodo, Priority: 1},
	)

	origFactory := semanticProviderFactory
	origFreshness := semanticFreshnessFn
	defer func() {
		semanticProviderFactory = origFactory
		semanticFreshnessFn = origFreshness
	}()
	semanticProviderFactory = func(cfg semantic.Config, opts semantic.ProviderOptions) (semantic.Provider, error) {
		return &testSemanticProvider{status: semantic.Status{
			Provider:  cfg.Provider,
			Model:     cfg.Model,
			Available: false,
			Details:   "uv missing",
		}}, nil
	}
	semanticFreshnessFn = func(ctx context.Context, repoRoot string, cfg semantic.Config) (semantic.Freshness, error) {
		return semantic.Freshness{Status: semantic.FreshnessMissing, Reason: "missing"}, nil
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"related", "TKT-001", "--semantic", "auto", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	var got relatedView
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatalf("json decode failed: %v", err)
	}
	if got.SemanticUsed {
		t.Fatalf("expected semantic fallback, got %#v", got)
	}
	if len(got.Warnings) == 0 {
		t.Fatalf("expected warnings, got %#v", got)
	}
}

func TestRelatedCmdSemanticOnFailsWhenUnavailable(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	relatedSemantic = "on"
	relatedLimit = 5

	cfg := ticket.DefaultConfig()
	cfg.Semantic.Enabled = true
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	seedRelatedTicketsWithoutConfig(t, tmpDir,
		&ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "semantic search", State: ticket.StateTodo, Priority: 1},
	)

	origFactory := semanticProviderFactory
	origFreshness := semanticFreshnessFn
	defer func() {
		semanticProviderFactory = origFactory
		semanticFreshnessFn = origFreshness
	}()
	semanticProviderFactory = func(cfg semantic.Config, opts semantic.ProviderOptions) (semantic.Provider, error) {
		return &testSemanticProvider{status: semantic.Status{
			Provider:  cfg.Provider,
			Model:     cfg.Model,
			Available: false,
			Details:   "uv missing",
		}}, nil
	}
	semanticFreshnessFn = func(ctx context.Context, repoRoot string, cfg semantic.Config) (semantic.Freshness, error) {
		return semantic.Freshness{Status: semantic.FreshnessMissing, Reason: "missing"}, nil
	}

	rootCmd.SetArgs([]string{"related", "TKT-001", "--semantic", "on"})
	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "semantic mode unavailable") {
		t.Fatalf("expected semantic failure, got %v", err)
	}
}

func TestRelatedCmdHybridUsesVectorScores(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	relatedSemantic = "auto"
	relatedLimit = 5

	cfg := ticket.DefaultConfig()
	cfg.Semantic.Enabled = true
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	seedRelatedTicketsWithoutConfig(t, tmpDir,
		&ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "semantic search", State: ticket.StateTodo, Priority: 1},
		&ticket.Ticket{ID: "TKT-002", Seq: 2, Title: "semantic search ranking", State: ticket.StateTodo, Priority: 1},
		&ticket.Ticket{ID: "TKT-003", Seq: 3, Title: "release notes", State: ticket.StateTodo, Priority: 1},
	)

	origFactory := semanticProviderFactory
	origFreshness := semanticFreshnessFn
	origVectorScoreFn := semanticVectorScoreFn
	defer func() {
		semanticProviderFactory = origFactory
		semanticFreshnessFn = origFreshness
		semanticVectorScoreFn = origVectorScoreFn
	}()
	semanticProviderFactory = func(cfg semantic.Config, opts semantic.ProviderOptions) (semantic.Provider, error) {
		return &testSemanticProvider{status: semantic.Status{
			Provider:  cfg.Provider,
			Model:     cfg.Model,
			Available: true,
		}}, nil
	}
	semanticFreshnessFn = func(ctx context.Context, repoRoot string, cfg semantic.Config) (semantic.Freshness, error) {
		return semantic.Freshness{Status: semantic.FreshnessFresh, Reason: "ok"}, nil
	}
	semanticVectorScoreFn = func(ctx context.Context, source *ticket.Ticket, cfg semantic.Config, provider semantic.Provider, repoRoot string, limit int) ([]semantic.VectorScore, error) {
		return []semantic.VectorScore{
			{TicketID: "TKT-003", Score: 1},
			{TicketID: "TKT-002", Score: 0.1},
		}, nil
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"related", "TKT-001", "--semantic", "auto", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var got relatedView
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatalf("json decode failed: %v", err)
	}
	if !got.SemanticUsed {
		t.Fatalf("expected semantic execution, got %#v", got)
	}
	if len(got.Results) == 0 || got.Results[0].TicketID != "TKT-003" {
		t.Fatalf("expected vector-heavy result first, got %#v", got.Results)
	}
}

func TestRelatedCmdEndToEndSemanticIndexing(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	relatedSemantic = "auto"
	relatedLimit = 5

	cfg := ticket.DefaultConfig()
	cfg.Semantic.Enabled = true
	cfg.Semantic.Provider = "fake"
	cfg.Semantic.Model = "fake-model"
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	seedRelatedTicketsWithoutConfig(t, tmpDir,
		&ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "semantic search", Description: "local vector index", State: ticket.StateTodo, Priority: 1},
		&ticket.Ticket{ID: "TKT-002", Seq: 2, Title: "semantic ranking", Description: "vector scoring", State: ticket.StateTodo, Priority: 1},
		&ticket.Ticket{ID: "TKT-003", Seq: 3, Title: "release notes", Description: "shipping docs", State: ticket.StateTodo, Priority: 1},
	)

	origFactory := semanticProviderFactory
	defer func() {
		semanticProviderFactory = origFactory
	}()
	semanticProviderFactory = func(cfg semantic.Config, opts semantic.ProviderOptions) (semantic.Provider, error) {
		return &semanticE2EProvider{}, nil
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"semantic", "rebuild", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("semantic rebuild failed: %v", err)
	}

	b.Reset()
	rootCmd.SetArgs([]string{"related", "TKT-001", "--semantic", "auto", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("related failed: %v", err)
	}

	var got relatedView
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatalf("json decode failed: %v", err)
	}
	if !got.SemanticUsed {
		t.Fatalf("expected semantic execution, got %#v", got)
	}
	if len(got.Results) == 0 || got.Results[0].TicketID != "TKT-002" {
		t.Fatalf("expected semantic neighbor first, got %#v", got.Results)
	}
}

func seedRelatedTickets(t *testing.T, repoRoot string, tickets ...*ticket.Ticket) {
	t.Helper()
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	seedRelatedTicketsWithoutConfig(t, repoRoot, tickets...)
}

func seedRelatedTicketsWithoutConfig(t *testing.T, repoRoot string, tickets ...*ticket.Ticket) {
	t.Helper()
	store := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	for _, tk := range tickets {
		if tk.CreatedAt.IsZero() {
			tk.CreatedAt = now
		}
		if tk.UpdatedAt.IsZero() {
			tk.UpdatedAt = now
		}
		if tk.CreatedBy == "" {
			tk.CreatedBy = "test"
		}
		if err := store.CreateTicket(context.Background(), tk); err != nil {
			t.Fatalf("CreateTicket failed: %v", err)
		}
	}
}
