package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanAnnotations_FindsAndSkips(t *testing.T) {
	repo := t.TempDir()

	mustWrite(t, filepath.Join(repo, "a.go"), "// [TKT-001] in go\n")
	mustWrite(t, filepath.Join(repo, "b.py"), "# [TKT-002] in py\n")
	mustWrite(t, filepath.Join(repo, "c.js"), "// [TKT-003] in js\n")
	mustWrite(t, filepath.Join(repo, "d.sh"), "# [TKT-004] in sh\n")

	mustWrite(t, filepath.Join(repo, ".git", "x.go"), "// [TKT-999] ignored\n")
	mustWrite(t, filepath.Join(repo, ".docket", "x.go"), "// [TKT-999] ignored\n")
	mustWrite(t, filepath.Join(repo, "vendor", "x.go"), "// [TKT-999] ignored\n")
	mustWrite(t, filepath.Join(repo, "node_modules", "x.js"), "// [TKT-999] ignored\n")
	mustWrite(t, filepath.Join(repo, ".svelte-kit", "x.js"), "// [TKT-999] ignored\n")
	mustWrite(t, filepath.Join(repo, "build", "x.js"), "// [TKT-999] ignored\n")
	mustWrite(t, filepath.Join(repo, "dist", "x.js"), "// [TKT-999] ignored\n")

	anns, err := ScanAnnotations(repo)
	if err != nil {
		t.Fatalf("ScanAnnotations failed: %v", err)
	}
	if len(anns) != 4 {
		t.Fatalf("annotations = %d, want 4", len(anns))
	}
}

func TestScanAnnotations_SkipsBinary(t *testing.T) {
	repo := t.TempDir()
	mustWrite(t, filepath.Join(repo, "ok.go"), "// [TKT-001] here\n")

	binPath := filepath.Join(repo, "bin.dat")
	if err := os.MkdirAll(filepath.Dir(binPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binPath, []byte{0x00, 0x01, 0x02, '[', 'T', 'K', 'T', '-', '9', '9', '9', ']'}, 0644); err != nil {
		t.Fatal(err)
	}

	anns, err := ScanAnnotations(repo)
	if err != nil {
		t.Fatalf("ScanAnnotations failed: %v", err)
	}
	if len(anns) != 1 || anns[0].TicketID != "TKT-001" {
		t.Fatalf("unexpected annotations: %+v", anns)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
