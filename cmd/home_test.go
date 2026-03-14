package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDocketHome_MissingEnv(t *testing.T) {
	t.Setenv("DOCKET_HOME", "")
	t.Setenv("HOME", t.TempDir())

	prevInteractive := docketHomeInteractiveFn
	defer func() { docketHomeInteractiveFn = prevInteractive }()
	docketHomeInteractiveFn = func() bool { return false }

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

func TestEnsureDocketHomeExportInShellRC_AppendsWhenMissing(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".zshrc")
	if err := os.WriteFile(rc, []byte("export PATH=/usr/bin\n"), 0o644); err != nil {
		t.Fatalf("seed rc failed: %v", err)
	}

	changed, err := ensureDocketHomeExportInShellRC(rc, "/tmp/docket-home")
	if err != nil {
		t.Fatalf("ensureDocketHomeExportInShellRC failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected rc file to be changed")
	}

	data, err := os.ReadFile(rc)
	if err != nil {
		t.Fatalf("read rc failed: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "export DOCKET_HOME=\"/tmp/docket-home\"") {
		t.Fatalf("expected DOCKET_HOME export in rc, got: %s", text)
	}
}

func TestEnsureDocketHomeExportInShellRC_SkipsExisting(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".bashrc")
	original := "export DOCKET_HOME=\"/already/set\"\n"
	if err := os.WriteFile(rc, []byte(original), 0o644); err != nil {
		t.Fatalf("seed rc failed: %v", err)
	}

	changed, err := ensureDocketHomeExportInShellRC(rc, "/tmp/docket-home")
	if err != nil {
		t.Fatalf("ensureDocketHomeExportInShellRC failed: %v", err)
	}
	if changed {
		t.Fatalf("expected rc update to be skipped when DOCKET_HOME already exists")
	}

	data, err := os.ReadFile(rc)
	if err != nil {
		t.Fatalf("read rc failed: %v", err)
	}
	if string(data) != original {
		t.Fatalf("expected rc content unchanged, got: %s", string(data))
	}
}

func TestResolveDocketHome_InteractiveBootstrapAccepted(t *testing.T) {
	t.Setenv("DOCKET_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	prevInteractive := docketHomeInteractiveFn
	prevPrompt := docketHomePromptFn
	defer func() {
		docketHomeInteractiveFn = prevInteractive
		docketHomePromptFn = prevPrompt
	}()

	docketHomeInteractiveFn = func() bool { return true }
	docketHomePromptFn = func(defaultPath string) (bool, error) { return true, nil }

	got, err := resolveDocketHome()
	if err != nil {
		t.Fatalf("resolveDocketHome failed: %v", err)
	}

	want := filepath.Join(home, ".docket-home")
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
	if os.Getenv("DOCKET_HOME") != want {
		t.Fatalf("expected DOCKET_HOME env to be set to %s, got %s", want, os.Getenv("DOCKET_HOME"))
	}
}

func TestResolveDocketHome_UsesShellRCWhenEnvMissing(t *testing.T) {
	t.Setenv("DOCKET_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	rc := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(rc, []byte("export DOCKET_HOME=\"$HOME/.docket-home\"\n"), 0o644); err != nil {
		t.Fatalf("write rc failed: %v", err)
	}

	prevInteractive := docketHomeInteractiveFn
	defer func() { docketHomeInteractiveFn = prevInteractive }()
	docketHomeInteractiveFn = func() bool { return false }

	got, err := resolveDocketHome()
	if err != nil {
		t.Fatalf("resolveDocketHome failed: %v", err)
	}
	want := filepath.Join(home, ".docket-home")
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
	if os.Getenv("DOCKET_HOME") != want {
		t.Fatalf("expected DOCKET_HOME env set from rc to %s, got %s", want, os.Getenv("DOCKET_HOME"))
	}
}
