package cmd

import (
	"path/filepath"
	"testing"
)

func TestRuntimeNamespaceRootPrefersRepoLocalNamespace(t *testing.T) {
	repoRoot := t.TempDir()
	repoLocal := defaultRuntimeNamespaceRoot(repoRoot)

	t.Run("without DOCKET_HOME", func(t *testing.T) {
		t.Setenv("DOCKET_HOME", "")
		docketHome = ""

		if got := runtimeNamespaceRoot(repoRoot); got != repoLocal {
			t.Fatalf("expected repo-local namespace root %q, got %q", repoLocal, got)
		}
	})

	t.Run("with DOCKET_HOME set", func(t *testing.T) {
		t.Setenv("DOCKET_HOME", filepath.Join(t.TempDir(), "external-docket-home"))
		docketHome = ""

		if got := runtimeNamespaceRoot(repoRoot); got != repoLocal {
			t.Fatalf("expected repo-local namespace root %q even when DOCKET_HOME is set, got %q", repoLocal, got)
		}
	})
}
