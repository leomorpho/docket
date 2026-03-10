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
	runner := &fakeRunner{result: CommandResult{Stdout: []byte("uv 0.7.0\n")}}
	provider := NewUVProvider(Config{Model: "sentence-transformers/all-MiniLM-L6-v2"}, ProviderOptions{RepoRoot: "/tmp/repo", Runner: runner})
	status, err := provider.Status(context.Background())
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status.Provider != "uv" || status.Model == "" {
		t.Fatalf("unexpected status: %+v", status)
	}
	if !status.Available {
		t.Fatalf("expected available status")
	}
	if status.Details != "uv 0.7.0" {
		t.Fatalf("unexpected details: %q", status.Details)
	}
	if runner.spec.Path != "uv" || strings.Join(runner.spec.Args, " ") != "--version" {
		t.Fatalf("unexpected status command: %#v", runner.spec)
	}
}

func TestUVProviderStatusUnavailable(t *testing.T) {
	runner := &fakeRunner{err: errors.New("executable file not found")}
	provider := NewUVProvider(Config{Model: "test-model"}, ProviderOptions{RepoRoot: "/tmp/repo", Runner: runner})
	status, err := provider.Status(context.Background())
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status.Available {
		t.Fatalf("expected unavailable status")
	}
	if !strings.Contains(status.Details, "executable file not found") {
		t.Fatalf("unexpected details: %q", status.Details)
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
	if want := []string{"run", "--no-project", "--with", "sentence-transformers==3.4.1", "python", "/tmp/repo/scripts/semantic_embed.py", "--model", "test-model"}; strings.Join(runner.spec.Args, "|") != strings.Join(want, "|") {
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

func TestUVProviderCommandEnvOverridesExisting(t *testing.T) {
	t.Setenv("HF_HOME", "/old/hf")
	t.Setenv("SENTENCE_TRANSFORMERS_HOME", "/old/sbert")
	t.Setenv("UV_CACHE_DIR", "/old/uv")
	provider := NewUVProvider(Config{
		HFHome:                   "/new/hf",
		SentenceTransformersHome: "/new/sbert",
		UVCacheDir:               "/new/uv",
	}, ProviderOptions{RepoRoot: "/tmp/repo"})
	env := provider.commandEnv()
	joined := strings.Join(env, "\n")
	for _, pair := range []string{"HF_HOME=/new/hf", "SENTENCE_TRANSFORMERS_HOME=/new/sbert", "UV_CACHE_DIR=/new/uv"} {
		if strings.Count(joined, pair) != 1 {
			t.Fatalf("expected single env entry %q in %q", pair, joined)
		}
	}
}

func TestUVPinnedPackages(t *testing.T) {
	if len(UVPinnedPackages) == 0 {
		t.Fatal("expected pinned packages")
	}
	if UVPinnedPackages[0] != "sentence-transformers==3.4.1" {
		t.Fatalf("unexpected package pin: %v", UVPinnedPackages)
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
