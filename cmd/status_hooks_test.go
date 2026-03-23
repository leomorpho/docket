package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/lifecycle"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestStatusIncludesQuietHookReadinessWhenHealthy(t *testing.T) {
	h := newFakeRepoHarness(t)
	if out, err := h.run("bootstrap", "--adapter", "codex"); err != nil {
		t.Fatalf("bootstrap failed: %v\n%s", err, out)
	}

	out, err := h.run("status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Hooks: ready") {
		t.Fatalf("expected ready hook summary, got:\n%s", out)
	}
	if strings.Contains(out, "Recent blocking hook events:") {
		t.Fatalf("healthy status should stay quiet about blocking events, got:\n%s", out)
	}
	if strings.Contains(out, "Hook remediation:") {
		t.Fatalf("healthy status should not print remediation, got:\n%s", out)
	}
	if !strings.Contains(out, "Security enforcement: warning-only") {
		t.Fatalf("expected warning-only enforcement note, got:\n%s", out)
	}
}

func TestStatusIncludesHookPolicyAndRecentBlockingEventsWhenDegraded(t *testing.T) {
	h := newFakeRepoHarness(t)
	if err := lifecycle.Append(h.repo, lifecycle.Event{
		Version:   lifecycle.SchemaVersionV1,
		Type:      lifecycle.EventToolFailure,
		EmittedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{
			"run_id":  "run-1",
			"command": "start",
			"phase":   "start_workflow",
			"tool":    "hooks.run_start",
			"error":   "start hook failed",
		},
	}); err != nil {
		t.Fatalf("append lifecycle hook failure failed: %v", err)
	}

	out, err := h.run("status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Hooks: needs-setup") {
		t.Fatalf("expected degraded hook readiness, got:\n%s", out)
	}
	if !strings.Contains(out, "Hook policy:") || !strings.Contains(out, "advisory") || !strings.Contains(out, "enforcement") {
		t.Fatalf("expected concise advisory/enforcement policy summary, got:\n%s", out)
	}
	if !strings.Contains(out, "Recent blocking hook events:") || !strings.Contains(out, "hooks.run_start") {
		t.Fatalf("expected recent blocking hook event details, got:\n%s", out)
	}
	if !strings.Contains(out, "Hook remediation: run `docket bootstrap`") {
		t.Fatalf("expected remediation guidance for degraded hooks, got:\n%s", out)
	}
}

func TestStatusReportsEnabledSecurityEnforcement(t *testing.T) {
	h := newFakeRepoHarness(t)
	cfg, err := ticket.LoadConfig(h.repo)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	cfg.SecurityEnforcement = true
	if err := ticket.SaveConfig(h.repo, cfg); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	out, err := h.run("status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Security enforcement: enabled") {
		t.Fatalf("expected enabled enforcement note, got:\n%s", out)
	}
}
