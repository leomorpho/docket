package semantic

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSemanticEmbedScriptWithFakeSentenceTransformers(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}

	root := repoRootFromWD(t)
	moduleDir := filepath.Join(t.TempDir(), "sentence_transformers")
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	module := `class SentenceTransformer:
    def __init__(self, model):
        self.model = model

    def encode(self, texts):
        return [[float(len(text)), float(i + 1)] for i, text in enumerate(texts)]
`
	if err := os.WriteFile(filepath.Join(moduleDir, "__init__.py"), []byte(module), 0644); err != nil {
		t.Fatalf("write module: %v", err)
	}

	payload := []byte(`{"inputs":[{"chunk_id":"c1","text":"hello"},{"chunk_id":"c2","text":"world!"}]}`)
	cmd := exec.Command(python, filepath.Join(root, "scripts", "semantic_embed.py"), "--model", "fake-model")
	cmd.Env = append(os.Environ(), "PYTHONPATH="+filepath.Dir(moduleDir))
	cmd.Stdin = bytes.NewReader(payload)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("script failed: %v stderr=%s", err, stderr.String())
	}

	var response EmbedResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Model != "fake-model" || response.Dimension != 2 || len(response.Results) != 2 {
		t.Fatalf("unexpected response: %+v", response)
	}
	if got := response.Results[0].Vector; len(got) != 2 || got[0] != 5 || got[1] != 1 {
		t.Fatalf("unexpected first vector: %#v", got)
	}
}

func TestSemanticEmbedScriptModelLoadError(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}

	root := repoRootFromWD(t)
	moduleDir := filepath.Join(t.TempDir(), "sentence_transformers")
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	module := `class SentenceTransformer:
    def __init__(self, model):
        raise RuntimeError("boom")
`
	if err := os.WriteFile(filepath.Join(moduleDir, "__init__.py"), []byte(module), 0644); err != nil {
		t.Fatalf("write module: %v", err)
	}

	payload := []byte(`{"inputs":[{"chunk_id":"c1","text":"hello"}]}`)
	cmd := exec.Command(python, filepath.Join(root, "scripts", "semantic_embed.py"), "--model", "fake-model")
	cmd.Env = append(os.Environ(), "PYTHONPATH="+filepath.Dir(moduleDir))
	cmd.Stdin = bytes.NewReader(payload)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err = cmd.Run()
	if err == nil {
		t.Fatal("expected script failure")
	}

	var response EmbedResponse
	if decodeErr := json.Unmarshal(stdout.Bytes(), &response); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if len(response.Errors) != 1 || response.Errors[0].Code != "model_load_error" {
		t.Fatalf("unexpected error response: %+v", response)
	}
}

func repoRootFromWD(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	return filepath.Dir(filepath.Dir(wd))
}
