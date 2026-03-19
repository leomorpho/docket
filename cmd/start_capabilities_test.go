package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	docketgit "github.com/leomorpho/docket/internal/git"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestRenderStartCapabilityDigest_HealthyAndDegraded(t *testing.T) {
	healthy := startCapabilityDigest{
		Adapter:     "codex",
		FlowPhases:  []string{"bootstrap", "start", "plan", "implement", "verify"},
		Readiness:   startReadiness{MCP: "ready", Skills: "ready", Hooks: "ready"},
		Remediation: "",
	}
	humanHealthy := renderStartCapabilityDigestHuman(healthy)
	if !strings.Contains(humanHealthy, "Flow: bootstrap -> start -> plan -> implement -> verify") {
		t.Fatalf("missing flow summary: %s", humanHealthy)
	}
	if !strings.Contains(humanHealthy, "Readiness: MCP=ready | Skills=ready | Hooks=ready") {
		t.Fatalf("missing healthy readiness strip: %s", humanHealthy)
	}
	if strings.Contains(humanHealthy, "docket bootstrap") {
		t.Fatalf("healthy snapshot should not include remediation: %s", humanHealthy)
	}

	degraded := healthy
	degraded.Readiness.Hooks = "needs-setup"
	degraded.Remediation = "Run `docket bootstrap` to install or repair integration artifacts."
	humanDegraded := renderStartCapabilityDigestHuman(degraded)
	if !strings.Contains(humanDegraded, "Readiness: MCP=ready | Skills=ready | Hooks=needs-setup") {
		t.Fatalf("missing degraded readiness strip: %s", humanDegraded)
	}
	if !strings.Contains(humanDegraded, "docket bootstrap") {
		t.Fatalf("expected remediation in degraded snapshot: %s", humanDegraded)
	}
}

func TestStartCapabilityDigestJSONSnapshot(t *testing.T) {
	digest := startCapabilityDigest{
		Adapter:    "codex",
		FlowPhases: []string{"bootstrap", "start", "plan", "implement", "verify"},
		Readiness: startReadiness{
			MCP:    "ready",
			Skills: "ready",
			Hooks:  "needs-setup",
		},
		Remediation: "Run `docket bootstrap` to install or repair integration artifacts.",
	}
	got, err := json.MarshalIndent(digest, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent failed: %v", err)
	}
	want := "{\n" +
		"  \"adapter\": \"codex\",\n" +
		"  \"flow_phases\": [\n" +
		"    \"bootstrap\",\n" +
		"    \"start\",\n" +
		"    \"plan\",\n" +
		"    \"implement\",\n" +
		"    \"verify\"\n" +
		"  ],\n" +
		"  \"readiness\": {\n" +
		"    \"mcp\": \"ready\",\n" +
		"    \"skills\": \"ready\",\n" +
		"    \"hooks\": \"needs-setup\"\n" +
		"  },\n" +
		"  \"remediation\": \"Run `docket bootstrap` to install or repair integration artifacts.\"\n" +
		"}"
	if string(got) != want {
		t.Fatalf("json snapshot mismatch\nwant:\n%s\ngot:\n%s", want, string(got))
	}
	t.Logf("json start capability digest snapshot:\n%s", string(got))
}

func TestStartOutputBeforeAndAfterBootstrap(t *testing.T) {
	tmpRepo := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	t.Setenv("DOCKET_AGENT_ID", "test-agent")
	docketHome = ""
	repo = tmpRepo
	format = "human"

	runGitSession(t, tmpRepo, "init")
	runGitSession(t, tmpRepo, "config", "user.email", "test@example.com")
	runGitSession(t, tmpRepo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(tmpRepo, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed file failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpRepo, "AGENTS.md"), []byte("codex marker"), 0o644); err != nil {
		t.Fatalf("write AGENTS marker failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpRepo, "doombox.json"), []byte(`{"mcp":"docket"}`), 0o644); err != nil {
		t.Fatalf("write doombox.json failed: %v", err)
	}
	runGitSession(t, tmpRepo, "add", ".")
	runGitSession(t, tmpRepo, "commit", "-m", "chore: seed")

	if err := ticket.SaveConfig(tmpRepo, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	s := local.New(tmpRepo)
	now := time.Now().UTC().Truncate(time.Second)
	for i := 1; i <= 2; i++ {
		id := fmt.Sprintf("TKT-%03d", i)
		if wt, err := docketgit.GetAgentWorktreeDir(tmpRepo, id); err == nil {
			_ = os.RemoveAll(wt)
		}
		if err := s.CreateTicket(context.Background(), &ticket.Ticket{
			ID:          id,
			Seq:         i,
			Title:       "ticket " + id,
			State:       ticket.State("todo"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "agent:test-agent",
			Description: "desc",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		}); err != nil {
			t.Fatalf("CreateTicket %s failed: %v", id, err)
		}
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"start"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("start before bootstrap failed: %v", err)
	}
	before := out.String()
	if !strings.Contains(before, "Hooks=needs-setup") {
		t.Fatalf("expected degraded hook readiness before bootstrap, got: %s", before)
	}
	if !strings.Contains(before, "Run `docket bootstrap`") {
		t.Fatalf("expected bootstrap remediation before bootstrap, got: %s", before)
	}
	t.Logf("start output before bootstrap:\n%s", before)

	out.Reset()
	rootCmd.SetArgs([]string{"bootstrap", "--adapter", "codex"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	out.Reset()
	rootCmd.SetArgs([]string{"start"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("start after bootstrap failed: %v", err)
	}
	after := out.String()
	if !strings.Contains(after, "Hooks=ready") {
		t.Fatalf("expected ready hooks after bootstrap, got: %s", after)
	}
	if strings.Contains(after, "Run `docket bootstrap`") {
		t.Fatalf("did not expect remediation after bootstrap, got: %s", after)
	}
	t.Logf("start output after bootstrap:\n%s", after)
}

func TestBuildStartCapabilityDigest_UnknownAdapterNeedsRemediation(t *testing.T) {
	repoRoot := t.TempDir()
	digest := buildStartCapabilityDigest(repoRoot)
	if digest.Adapter != "unsupported" {
		t.Fatalf("expected unsupported adapter, got %q", digest.Adapter)
	}
	if digest.Readiness.MCP != "needs-setup" || digest.Readiness.Skills != "needs-setup" || digest.Readiness.Hooks != "needs-setup" {
		t.Fatalf("expected full degraded readiness for unknown adapter, got %#v", digest.Readiness)
	}
	if !strings.Contains(digest.Remediation, "docket bootstrap") {
		t.Fatalf("expected remediation hint, got %q", digest.Remediation)
	}
}

func TestBuildStartCapabilityDigest_ClaudeSkillsReadyWhenManagedBlockExists(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir hooks failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "CLAUDE.md"), []byte(claudeManagedBlock(repoRoot)), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md failed: %v", err)
	}
	if _, err := writeHook(repoRoot); err != nil {
		t.Fatalf("writeHook failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "doombox.json"), []byte(`{"mcp":"docket"}`), 0o644); err != nil {
		t.Fatalf("write doombox.json failed: %v", err)
	}

	digest := buildStartCapabilityDigest(repoRoot)
	if digest.Adapter != "claude-code" {
		t.Fatalf("expected claude-code adapter, got %q", digest.Adapter)
	}
	if digest.Readiness.Skills != "ready" || digest.Readiness.Hooks != "ready" || digest.Readiness.MCP != "ready" {
		t.Fatalf("expected all readiness checks to be ready, got %#v", digest.Readiness)
	}
}
