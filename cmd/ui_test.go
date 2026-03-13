package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRegistration(t *testing.T) {
	// 1. Mock Hub Server
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "service": "docket-ui"})
	})
	mux.HandleFunc("/api/projects/register", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["dir"] == "" {
			w.WriteHeader(400)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"project": map[string]string{"id": "test-project"},
		})
	})
	
	server := httptest.NewServer(mux)
	defer server.Close()

	// 2. Test registerWithHub
	id, ok := registerWithHub(server.URL, "/some/dir")
	if !ok || id != "test-project" {
		t.Errorf("registerWithHub failed: id=%s, ok=%v", id, ok)
	}

	// 3. Test postRegister directly
	id, ok = postRegister(server.URL, "/some/dir", time.Second)
	if !ok || id != "test-project" {
		t.Errorf("postRegister failed: id=%s, ok=%v", id, ok)
	}
}

func TestParseOpenAnswer(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"y\n", true},
		{"Y\n", true},
		{"yes\n", true},
		{"Yes\n", true},
		{"\n", true},
		{"n\n", false},
		{"no\n", false},
		{"junk\n", false},
	}

	for _, tt := range tests {
		if got := parseOpenAnswer(tt.input); got != tt.expected {
			t.Errorf("parseOpenAnswer(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestResolveWebDir(t *testing.T) {
	tmpDir := t.TempDir()
	webDir := filepath.Join(tmpDir, "web")
	os.MkdirAll(webDir, 0755)
	os.WriteFile(filepath.Join(webDir, "package.json"), []byte(`{"name": "docket-ui"}`), 0644)

	// 1. Using env var
	os.Setenv("DOCKET_UI_DIR", webDir)
	defer os.Unsetenv("DOCKET_UI_DIR")
	got, err := resolveWebDir()
	if err != nil {
		t.Fatalf("resolveWebDir with env failed: %v", err)
	}
	realGotEnv, _ := filepath.EvalSymlinks(got)
	realWebEnv, _ := filepath.EvalSymlinks(webDir)
	if realGotEnv != realWebEnv {
		t.Errorf("expected %s, got %s", realWebEnv, realGotEnv)
	}

	// 2. Walk up from current dir
	os.Unsetenv("DOCKET_UI_DIR")
	oldRepo := repo
	repo = ""
	defer func() { repo = oldRepo }()
	cwd, _ := os.Getwd()
	os.Chdir(webDir)
	defer os.Chdir(cwd)
	got, err = resolveWebDir()
	if err != nil {
		t.Fatalf("resolveWebDir from cwd failed: %v", err)
	}
	realGot, _ := filepath.EvalSymlinks(got)
	realWeb, _ := filepath.EvalSymlinks(webDir)
	if realGot != realWeb {
		t.Errorf("expected %s, got %s", realWeb, realGot)
	}

	// 3. Test invalid package name rejection
	invalidDir := filepath.Join(tmpDir, "invalid-web")
	os.MkdirAll(invalidDir, 0755)
	os.WriteFile(filepath.Join(invalidDir, "package.json"), []byte(`{"name": "not-docket"}`), 0644)
	os.Setenv("DOCKET_UI_DIR", invalidDir)
	_, err = resolveWebDir()
	if err == nil {
		t.Error("expected error for invalid package name, got nil")
	}
}

func TestStreamPipe(t *testing.T) {
	input := "Line 1\nLocal: http://127.0.0.1:5173/\nLine 3\n"
	r := bytes.NewReader([]byte(input))
	w := new(bytes.Buffer)
	localURLCh := make(chan string, 1)
	var wg sync.WaitGroup
	wg.Add(1)

	streamPipe(r, w, true, localURLCh, &wg)
	wg.Wait()

	if !bytes.Contains(w.Bytes(), []byte("Line 1")) {
		t.Error("expected Line 1 in output")
	}
	select {
	case url := <-localURLCh:
		if url != "http://127.0.0.1:5173/" {
			t.Errorf("expected http://127.0.0.1:5173/, got %s", url)
		}
	default:
		t.Error("expected URL to be detected")
	}
}
