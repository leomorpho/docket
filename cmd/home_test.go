package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDocketHome_MissingEnv(t *testing.T) {
	t.Setenv("DOCKET_HOME", "")

	_, err := resolveDocketHome()
	if err == nil {
		t.Fatal("expected error when DOCKET_HOME is unset")
	}
	if !strings.Contains(err.Error(), "DOCKET_HOME is required") {
		t.Fatalf("unexpected error message: %v", err)
	}
	if !strings.Contains(err.Error(), "DOCKET_HOME=") {
		t.Fatalf("error message should include example path, got: %v", err)
	}
}

func TestResolveDocketHome_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "secure-root")
	t.Setenv("DOCKET_HOME", dir)

	got, err := resolveDocketHome()
	if err != nil {
		t.Fatalf("resolveDocketHome failed: %v", err)
	}

	abs, _ := filepath.Abs(dir)
	if got != abs {
		t.Fatalf("expected %s, got %s", abs, got)
	}
	if fi, err := os.Stat(got); err != nil || !fi.IsDir() {
		t.Fatalf("expected directory %s to exist, stat error: %v", got, err)
	}
}
