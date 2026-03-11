package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/leoaudibert/docket/internal/semantic"
	"github.com/leoaudibert/docket/internal/ticket"
)

type testSemanticProvider struct {
	status semantic.Status
}

func (p *testSemanticProvider) Name() string { return p.status.Provider }
func (p *testSemanticProvider) Status(context.Context) (semantic.Status, error) {
	return p.status, nil
}
func (p *testSemanticProvider) Embed(context.Context, semantic.EmbedRequest) (semantic.EmbedResponse, error) {
	return semantic.EmbedResponse{}, nil
}

func TestSemanticStatusCmd(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	cfg := ticket.DefaultConfig()
	cfg.Semantic.Enabled = true
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	meta := semantic.NewMetadata()
	meta.Provider = "uv"
	meta.Model = "sentence-transformers/all-MiniLM-L6-v2"
	meta.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	meta.UpsertChunk(semantic.ChunkMetadata{ChunkID: "TKT-001:title", TicketID: "TKT-001", Type: semantic.ChunkTypeTitle, Hash: "h"})
	if err := semantic.SaveMetadata(tmpDir, meta); err != nil {
		t.Fatalf("SaveMetadata failed: %v", err)
	}

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
			Available: true,
			Details:   "uv 0.7.0",
		}}, nil
	}
	semanticFreshnessFn = func(ctx context.Context, repoRoot string, cfg semantic.Config) (semantic.Freshness, error) {
		return semantic.Freshness{Status: semantic.FreshnessFresh, Reason: "ok"}, nil
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"semantic", "status"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatalf("json decode failed: %v", err)
	}
	if got["provider"] != "uv" || got["model"] == "" {
		t.Fatalf("unexpected status payload: %v", got)
	}
	index := got["index"].(map[string]interface{})
	if index["chunk_count"].(float64) != 1 || index["ticket_count"].(float64) != 1 {
		t.Fatalf("unexpected index payload: %v", index)
	}
}

func TestSemanticRebuildCmd(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	origFactory := semanticProviderFactory
	origIncremental := semanticIncrementalFn
	origFull := semanticFullFn
	defer func() {
		semanticProviderFactory = origFactory
		semanticIncrementalFn = origIncremental
		semanticFullFn = origFull
		semanticRebuildFull = false
	}()

	semanticProviderFactory = func(cfg semantic.Config, opts semantic.ProviderOptions) (semantic.Provider, error) {
		return &testSemanticProvider{status: semantic.Status{Provider: "uv", Model: cfg.Model, Available: true}}, nil
	}
	semanticIncrementalFn = func(ctx context.Context, repoRoot string, provider semantic.Provider, cfg semantic.Config) (semantic.RebuildStats, error) {
		return semantic.RebuildStats{Added: 1, Changed: 2, Deleted: 3, Unchanged: 4}, nil
	}
	semanticFullFn = func(ctx context.Context, repoRoot string, provider semantic.Provider, cfg semantic.Config) (semantic.RebuildStats, error) {
		return semantic.RebuildStats{Added: 9, Changed: 0, Deleted: 0, Unchanged: 0}, nil
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"semantic", "rebuild"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute incremental failed: %v", err)
	}
	if !strings.Contains(b.String(), "incremental") || !strings.Contains(b.String(), "added=1") {
		t.Fatalf("unexpected incremental output: %q", b.String())
	}

	format = "json"
	b.Reset()
	rootCmd.SetArgs([]string{"semantic", "rebuild", "--full"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute full failed: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatalf("json decode failed: %v", err)
	}
	if got["mode"] != "full" || got["added"].(float64) != 9 {
		t.Fatalf("unexpected full rebuild payload: %v", got)
	}
}
