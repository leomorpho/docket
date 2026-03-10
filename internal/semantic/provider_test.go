package semantic

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type fakeRunner struct {
	spec   CommandSpec
	result CommandResult
	err    error
}

func (f *fakeRunner) Run(ctx context.Context, spec CommandSpec) (CommandResult, error) {
	f.spec = spec
	return f.result, f.err
}

func TestNewProvider(t *testing.T) {
	provider, err := NewProvider(Config{Provider: "uv", Model: "model"}, ProviderOptions{RepoRoot: "/tmp/repo"})
	if err != nil {
		t.Fatalf("NewProvider failed: %v", err)
	}
	if provider.Name() != "uv" {
		t.Fatalf("provider name = %q", provider.Name())
	}
}

func TestNewProvider_Unknown(t *testing.T) {
	_, err := NewProvider(Config{Provider: "bogus"}, ProviderOptions{})
	if err == nil {
		t.Fatal("expected unknown provider error")
	}
	if !errors.Is(err, ErrUnknownProvider) {
		t.Fatalf("expected ErrUnknownProvider, got %v", err)
	}
}

func TestUVProviderStatus(t *testing.T) {
	provider := NewUVProvider(Config{Model: "sentence-transformers/all-MiniLM-L6-v2"}, ProviderOptions{RepoRoot: "/tmp/repo"})
	status, err := provider.Status(context.Background())
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status.Provider != "uv" || status.Model == "" {
		t.Fatalf("unexpected status: %+v", status)
	}
	if status.Available {
		t.Fatalf("expected placeholder unavailable status")
	}
}

func TestUVProviderEmbedInvokesBridge(t *testing.T) {
	response := EmbedResponse{
		Model:     "test-model",
		Dimension: 0,
		Results: []EmbedResult{
			{ChunkID: "chunk-1", Vector: []float64{}},
		},
	}
	stdout, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	runner := &fakeRunner{result: CommandResult{Stdout: stdout}}
	provider := NewUVProvider(Config{
		Model:                    "test-model",
		HFHome:                   "/tmp/hf",
		SentenceTransformersHome: "/tmp/sbert",
		UVCacheDir:               "/tmp/uv",
	}, ProviderOptions{
		RepoRoot: "/tmp/repo",
		Runner:   runner,
	})

	got, err := provider.Embed(context.Background(), EmbedRequest{
		Model: "test-model",
		Inputs: []Input{{
			ChunkID:  "chunk-1",
			TicketID: "TKT-001",
			Field:    "title",
			Text:     "hello",
		}},
	})
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	if got.Model != "test-model" || len(got.Results) != 1 {
		t.Fatalf("unexpected response: %+v", got)
	}
	if runner.spec.Path != "uv" {
		t.Fatalf("expected uv path, got %q", runner.spec.Path)
	}
	if want := []string{"run", "--no-project", "python", "/tmp/repo/scripts/semantic_embed.py", "--model", "test-model"}; strings.Join(runner.spec.Args, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected args: %v", runner.spec.Args)
	}
	if runner.spec.Dir != "/tmp/repo" {
		t.Fatalf("unexpected dir: %q", runner.spec.Dir)
	}
	stdin := string(runner.spec.Stdin)
	if !strings.Contains(stdin, "\"chunk_id\":\"chunk-1\"") {
		t.Fatalf("stdin missing chunk payload: %s", stdin)
	}
	env := strings.Join(runner.spec.Env, "\n")
	for _, needle := range []string{"HF_HOME=/tmp/hf", "SENTENCE_TRANSFORMERS_HOME=/tmp/sbert", "UV_CACHE_DIR=/tmp/uv"} {
		if !strings.Contains(env, needle) {
			t.Fatalf("env missing %q", needle)
		}
	}
}

func TestUVProviderEmbedInvalidJSON(t *testing.T) {
	runner := &fakeRunner{result: CommandResult{Stdout: []byte("not-json")}}
	provider := NewUVProvider(Config{Model: "m"}, ProviderOptions{RepoRoot: "/tmp/repo", Runner: runner})
	_, err := provider.Embed(context.Background(), EmbedRequest{})
	if err == nil || !strings.Contains(err.Error(), "decode bridge response") {
		t.Fatalf("expected decode error, got %v", err)
	}
}
