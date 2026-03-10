package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanAndRefs(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	mustWriteScan(t, filepath.Join(tmpDir, "internal", "a.go"), "// [TKT-001] hello\n")
	mustWriteScan(t, filepath.Join(tmpDir, "script.py"), "# [TKT-001] py\n# [TKT-002] also\n")
	mustWriteScan(t, filepath.Join(tmpDir, "node_modules", "x.js"), "// [TKT-999] ignored\n")

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"scan"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if !strings.Contains(b.String(), "Found 3 annotations across 2 tickets") {
		t.Fatalf("unexpected scan output:\n%s", b.String())
	}

	b.Reset()
	rootCmd.SetArgs([]string{"refs", "TKT-001"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("refs failed: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "TKT-001 referenced in 2 locations") {
		t.Fatalf("unexpected refs output:\n%s", out)
	}
	if !strings.Contains(out, "internal/a.go:1") || !strings.Contains(out, "script.py:1") {
		t.Fatalf("missing refs:\n%s", out)
	}

	b.Reset()
	rootCmd.SetArgs([]string{"refs", "TKT-999"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("refs unknown failed: %v", err)
	}
	if !strings.Contains(b.String(), "No annotations found") {
		t.Fatalf("unexpected unknown refs output:\n%s", b.String())
	}
}

func TestScanUpsert_NoDuplicates(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"

	mustWriteScan(t, filepath.Join(tmpDir, "main.go"), "// [TKT-001] first\n")

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"scan", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("first scan failed: %v", err)
	}

	b.Reset()
	rootCmd.SetArgs([]string{"scan", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("second scan failed: %v", err)
	}

	b.Reset()
	rootCmd.SetArgs([]string{"refs", "TKT-001", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("refs json failed: %v", err)
	}

	var res struct {
		References []map[string]interface{} `json:"references"`
	}
	if err := json.Unmarshal(b.Bytes(), &res); err != nil {
		t.Fatalf("json decode failed: %v", err)
	}
	if len(res.References) != 1 {
		t.Fatalf("references = %d, want 1", len(res.References))
	}
}

func mustWriteScan(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
