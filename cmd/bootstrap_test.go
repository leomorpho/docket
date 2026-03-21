package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leomorpho/docket/internal/adapters"
)

func TestExecuteBootstrapOrchestrationOrder(t *testing.T) {
	order := []string{}
	deps := bootstrapDeps{
		resolve: func(_, _ string) adapters.Adapter {
			order = append(order, "resolve")
			return successAdapter{id: "codex"}
		},
		coreInstall: func(string) (bool, error) {
			order = append(order, "core")
			return true, nil
		},
		runInstall: func(context.Context, adapters.Adapter, adapters.InstallInput) error {
			order = append(order, "install")
			return nil
		},
		runBootstrap: func(context.Context, adapters.Adapter, adapters.BootstrapInput) error {
			order = append(order, "bootstrap")
			return nil
		},
	}

	out, err := executeBootstrap(context.Background(), "/tmp/repo", "codex", deps)
	if err != nil {
		t.Fatalf("executeBootstrap failed: %v", err)
	}
	if got, want := strings.Join(order, ","), "resolve,core,install,bootstrap"; got != want {
		t.Fatalf("unexpected order: got %q want %q", got, want)
	}
	if len(out.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(out.Steps))
	}
}

func TestExecuteBootstrapStepErrorBubbles(t *testing.T) {
	deps := bootstrapDeps{
		resolve: func(_, _ string) adapters.Adapter { return successAdapter{id: "codex"} },
		coreInstall: func(string) (bool, error) {
			return true, nil
		},
		runInstall: func(context.Context, adapters.Adapter, adapters.InstallInput) error {
			return errors.New("install exploded")
		},
		runBootstrap: adapters.RunBootstrap,
	}

	_, err := executeBootstrap(context.Background(), "/tmp/repo", "codex", deps)
	if err == nil || !strings.Contains(err.Error(), "install exploded") {
		t.Fatalf("expected install error to bubble, got %v", err)
	}
}

func TestExecuteBootstrapIdempotentRerunBehavior(t *testing.T) {
	calls := 0
	deps := bootstrapDeps{
		resolve: func(_, _ string) adapters.Adapter { return successAdapter{id: "codex"} },
		coreInstall: func(string) (bool, error) {
			calls++
			return calls == 1, nil
		},
		runInstall:   adapters.RunInstall,
		runBootstrap: adapters.RunBootstrap,
	}

	first, err := executeBootstrap(context.Background(), "/tmp/repo", "codex", deps)
	if err != nil {
		t.Fatalf("first executeBootstrap failed: %v", err)
	}
	second, err := executeBootstrap(context.Background(), "/tmp/repo", "codex", deps)
	if err != nil {
		t.Fatalf("second executeBootstrap failed: %v", err)
	}

	if first.Steps[0].Status != "changed" {
		t.Fatalf("expected first run core step to be changed, got %q", first.Steps[0].Status)
	}
	if second.Steps[0].Status != "no-change" {
		t.Fatalf("expected second run core step to be no-change, got %q", second.Steps[0].Status)
	}
}

func TestBootstrapCommandRunsTwiceNoOpSafe(t *testing.T) {
	tmpRepo := t.TempDir()
	t.Setenv("DOCKET_HOME", filepath.Join(t.TempDir(), "docket-home"))
	docketHome = ""
	repo = tmpRepo
	format = "human"

	if err := os.MkdirAll(filepath.Join(tmpRepo, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir .git/hooks failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpRepo, "AGENTS.md"), []byte("marker"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md failed: %v", err)
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"bootstrap", "--adapter", "codex"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("first bootstrap failed: %v", err)
	}
	first := out.String()
	inventoryAfterFirst := append([]string{}, starterScaffoldManagedArtifacts(tmpRepo)...)
	inventoryAfterFirst = append(inventoryAfterFirst, installManifestPath(tmpRepo))
	if !strings.Contains(first, "core install: changed") {
		t.Fatalf("expected changed core install step, got: %s", first)
	}
	if !strings.Contains(first, "Run `docket start`") {
		t.Fatalf("expected next-step instruction, got: %s", first)
	}

	out.Reset()
	rootCmd.SetArgs([]string{"bootstrap", "--adapter", "codex"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("second bootstrap failed: %v", err)
	}
	second := out.String()
	if !strings.Contains(second, "core install: no-change") {
		t.Fatalf("expected no-change on second run, got: %s", second)
	}
	inventoryAfterSecond := append([]string{}, starterScaffoldManagedArtifacts(tmpRepo)...)
	inventoryAfterSecond = append(inventoryAfterSecond, installManifestPath(tmpRepo))
	gitignoreData, err := os.ReadFile(filepath.Join(tmpRepo, ".gitignore"))
	if err != nil {
		t.Fatalf("gitignore missing after bootstrap: %v", err)
	}
	if !strings.Contains(string(gitignoreData), ".docket/local/") {
		t.Fatalf("expected bootstrap to reconcile canonical local gitignore entry, got:\n%s", string(gitignoreData))
	}
	t.Logf("bootstrap summary first run:\n%s", first)
	t.Logf("bootstrap summary second run:\n%s", second)
	t.Logf("artifact inventory after first run: %s", strings.Join(inventoryAfterFirst, ", "))
	t.Logf("artifact inventory after second run: %s", strings.Join(inventoryAfterSecond, ", "))
}

func TestBootstrapCommandOverrideReflectedInJSONOutput(t *testing.T) {
	tmpRepo := t.TempDir()
	t.Setenv("DOCKET_HOME", filepath.Join(t.TempDir(), "docket-home"))
	t.Setenv("DOCKET_ADAPTER", "")
	docketHome = ""
	repo = tmpRepo
	format = "json"

	if err := os.MkdirAll(filepath.Join(tmpRepo, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir .git/hooks failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpRepo, "AGENTS.md"), []byte("codex marker"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpRepo, "CLAUDE.md"), []byte("claude marker"), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md failed: %v", err)
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"bootstrap", "--adapter", "gemini"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("bootstrap --adapter gemini failed: %v\n%s", err, out.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("parse bootstrap json failed: %v\n%s", err, out.String())
	}
	adapterObj, ok := payload["adapter"].(map[string]any)
	if !ok {
		t.Fatalf("expected adapter object in payload: %#v", payload)
	}
	if adapterObj["id"] != "gemini" {
		t.Fatalf("expected adapter id gemini, got %v", adapterObj["id"])
	}
	if payload["adapter_source"] != "override" {
		t.Fatalf("expected adapter_source override, got %v", payload["adapter_source"])
	}
}

func TestBootstrapCommandAmbiguityWarningInHumanOutput(t *testing.T) {
	tmpRepo := t.TempDir()
	t.Setenv("DOCKET_HOME", filepath.Join(t.TempDir(), "docket-home"))
	t.Setenv("DOCKET_ADAPTER", "")
	docketHome = ""
	repo = tmpRepo
	format = "human"

	if err := os.MkdirAll(filepath.Join(tmpRepo, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir .git/hooks failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpRepo, "AGENTS.md"), []byte("codex marker"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpRepo, "CLAUDE.md"), []byte("claude marker"), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md failed: %v", err)
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"bootstrap"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("bootstrap failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Adapter warning: ambiguous adapter detection") {
		t.Fatalf("expected ambiguity warning in output, got:\n%s", out.String())
	}
}

type successAdapter struct {
	id string
}

func (s successAdapter) Metadata() adapters.Metadata {
	return adapters.Metadata{ID: s.id}
}

func (s successAdapter) Detect(_ string) bool {
	return true
}

func (s successAdapter) Bootstrap(_ context.Context, _ adapters.BootstrapInput) error {
	return nil
}

func (s successAdapter) Doctor(_ context.Context, _ string) (adapters.DoctorReport, error) {
	return adapters.DoctorReport{}, nil
}

func (s successAdapter) Status(_ context.Context, _ string) (adapters.StatusReport, error) {
	return adapters.StatusReport{Ready: true}, nil
}

func (s successAdapter) Install(_ context.Context, _ adapters.InstallInput) error {
	return nil
}
