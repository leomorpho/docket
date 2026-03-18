package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/semantic"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestSearchCmdLexicalOnly(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	searchSemantic = "off"
	searchLimit = 10

	seedRelatedTickets(t, tmpDir,
		&ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "auth middleware", Description: "jwt validation for api routes", State: ticket.State("todo"), Priority: 1},
		&ticket.Ticket{ID: "TKT-002", Seq: 2, Title: "release notes", Description: "docs only", State: ticket.State("todo"), Priority: 1},
	)

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"search", "jwt validation", "--semantic", "off", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var got searchView
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatalf("json decode failed: %v", err)
	}
	if got.SemanticUsed {
		t.Fatalf("expected lexical-only execution, got %#v", got)
	}
	if len(got.Results) == 0 || got.Results[0].TicketID != "TKT-001" {
		t.Fatalf("unexpected search results: %#v", got.Results)
	}
}

func TestSearchCmdIDMatchBoostsExactTicket(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	searchSemantic = "off"
	searchLimit = 10

	seedRelatedTickets(t, tmpDir,
		&ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "auth middleware", Description: "jwt validation", State: ticket.State("todo"), Priority: 1},
		&ticket.Ticket{ID: "TKT-010", Seq: 10, Title: "another auth task", Description: "jwt validation", State: ticket.State("todo"), Priority: 1},
	)

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"search", "TKT-010", "--semantic", "off", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var got searchView
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatalf("json decode failed: %v", err)
	}
	if len(got.Results) == 0 || got.Results[0].TicketID != "TKT-010" {
		t.Fatalf("expected exact ID match first, got %#v", got.Results)
	}
}

func TestSearchCmdHybridUsesVectorScores(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	searchSemantic = "auto"
	searchLimit = 10

	cfg := ticket.DefaultConfig()
	cfg.Semantic.Enabled = true
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	seedRelatedTicketsWithoutConfig(t, tmpDir,
		&ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "auth middleware", State: ticket.State("todo"), Priority: 1},
		&ticket.Ticket{ID: "TKT-002", Seq: 2, Title: "release notes", State: ticket.State("todo"), Priority: 1},
	)

	origFactory := semanticProviderFactory
	origFreshness := semanticFreshnessFn
	origQueryVectorScoreFn := semanticQueryVectorScoreFn
	defer func() {
		semanticProviderFactory = origFactory
		semanticFreshnessFn = origFreshness
		semanticQueryVectorScoreFn = origQueryVectorScoreFn
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
	semanticQueryVectorScoreFn = func(ctx context.Context, query string, cfg semantic.Config, provider semantic.Provider, repoRoot string, limit int) ([]semantic.VectorScore, error) {
		return []semantic.VectorScore{
			{TicketID: "TKT-002", Score: 1},
			{TicketID: "TKT-001", Score: 0.1},
		}, nil
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"search", "semantic vector stuff", "--semantic", "auto", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var got searchView
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatalf("json decode failed: %v", err)
	}
	if !got.SemanticUsed {
		t.Fatalf("expected semantic execution, got %#v", got)
	}
	if len(got.Results) == 0 || got.Results[0].TicketID != "TKT-002" {
		t.Fatalf("expected vector-heavy result first, got %#v", got.Results)
	}
}

func TestTicketRenderIncludesEditingGuidance(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	tkt := &ticket.Ticket{
		ID:          "TKT-123",
		Seq:         123,
		Title:       "Rendered guidance",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
		Description: "Long enough description to satisfy validation and exercise ticket rendering behavior for direct edits.",
		AC: []ticket.AcceptanceCriterion{
			{Description: "Acceptance criterion one"},
		},
	}

	store := local.New(t.TempDir())
	if err := store.CreateTicket(context.Background(), tkt); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}
	raw, err := store.GetRaw(context.Background(), "TKT-123")
	if err != nil {
		t.Fatalf("GetRaw failed: %v", err)
	}
	if !strings.Contains(raw, "Direct edits are allowed") {
		t.Fatalf("expected direct-edit guidance in raw ticket, got:\n%s", raw)
	}
	if !strings.Contains(raw, "`docket validate TKT-123`") {
		t.Fatalf("expected ticket-specific validate guidance in raw ticket, got:\n%s", raw)
	}
}
