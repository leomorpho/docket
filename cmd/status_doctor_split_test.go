package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestStatusAndDoctorDescriptionsStayDistinct(t *testing.T) {
	if !strings.Contains(strings.ToLower(statusCmd.Short), "runtime") {
		t.Fatalf("status short description must focus runtime state, got: %q", statusCmd.Short)
	}
	if strings.Contains(strings.ToLower(statusCmd.Short), "integration health") {
		t.Fatalf("status short should not overlap doctor framing, got: %q", statusCmd.Short)
	}

	if !strings.Contains(strings.ToLower(doctorCmd.Short), "integration health") {
		t.Fatalf("doctor short description must focus integration health, got: %q", doctorCmd.Short)
	}
	if strings.Contains(strings.ToLower(doctorCmd.Short), "parallel safety") {
		t.Fatalf("doctor short should not overlap status framing, got: %q", doctorCmd.Short)
	}
}

func TestStatusAndDoctorOutputScopesStayDistinct(t *testing.T) {
	h := newFakeRepoHarness(t)

	statusOut, err := h.run("status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, "Runtime status:") {
		t.Fatalf("status output should announce runtime-state scope, got:\n%s", statusOut)
	}
	if strings.Contains(strings.ToLower(statusOut), "parallel") {
		t.Fatalf("status output should stay serial-first and omit parallel guidance, got:\n%s", statusOut)
	}
	if strings.Contains(statusOut, "Security enforcement:") {
		t.Fatalf("status output should stay on runtime state and omit legacy security framing, got:\n%s", statusOut)
	}

	doctorOut, err := h.run("doctor")
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, doctorOut)
	}
	if !strings.Contains(doctorOut, "Setup and integration health") {
		t.Fatalf("doctor output should announce setup/integration-health scope, got:\n%s", doctorOut)
	}
	if strings.Contains(strings.ToLower(doctorOut), "parallel matrix") {
		t.Fatalf("doctor output should not overlap runtime parallel matrix wording, got:\n%s", doctorOut)
	}
}

func TestStatusHelpOmitsParallelFlag(t *testing.T) {
	h := newFakeRepoHarness(t)

	out, err := h.run("status", "--help")
	if err != nil {
		t.Fatalf("status --help failed: %v\n%s", err, out)
	}
	if strings.Contains(out, "--parallel") {
		t.Fatalf("status help should not expose retired parallel flag, got:\n%s", out)
	}
}

func TestStatusRejectsParallelFlag(t *testing.T) {
	h := newFakeRepoHarness(t)

	out, err := h.run("status", "--parallel")
	if err == nil {
		t.Fatalf("expected retired --parallel flag to fail")
	}
	if !strings.Contains(out, "unknown flag: --parallel") {
		t.Fatalf("expected unknown flag error for retired parallel view, got:\n%s", out)
	}
}

func TestStatusReportsOnlyRunnableLeafFromSharedQueueDefinition(t *testing.T) {
	h := newFakeRepoHarness(t)

	s := local.New(h.repo)
	now := time.Now().UTC().Truncate(time.Second)
	for _, tk := range []*ticket.Ticket{
		{
			ID:          "TKT-950",
			Seq:         950,
			Title:       "Blocker",
			State:       ticket.State("running"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: updateRunnableDescription(),
			AC:          updateRunnableAC(),
		},
		{
			ID:          "TKT-951",
			Seq:         951,
			Title:       "Blocked ready ticket",
			State:       ticket.State("ready"),
			Priority:    2,
			BlockedBy:   []string{"TKT-950"},
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: updateRunnableDescription(),
			AC:          updateRunnableAC(),
		},
		{
			ID:          "TKT-952",
			Seq:         952,
			Title:       "Runnable ready ticket",
			State:       ticket.State("ready"),
			Priority:    3,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: updateRunnableDescription(),
			AC:          updateRunnableAC(),
		},
	} {
		if err := s.CreateTicket(context.Background(), tk); err != nil {
			t.Fatalf("create %s: %v", tk.ID, err)
		}
	}

	out, err := h.run("status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "next runnable ticket is TKT-952") {
		t.Fatalf("expected status to report the same runnable leaf as start/selector, got:\n%s", out)
	}
	if strings.Contains(out, "TKT-951") {
		t.Fatalf("status should not present blocked ready tickets as runnable, got:\n%s", out)
	}
}

func TestDoctorReportsSecurityEnforcementMode(t *testing.T) {
	h := newFakeRepoHarness(t)

	warnOut, err := h.run("doctor")
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, warnOut)
	}
	if !strings.Contains(warnOut, "Security enforcement: warning-only") {
		t.Fatalf("expected warning-only enforcement note, got:\n%s", warnOut)
	}

	cfg, err := ticket.LoadConfig(h.repo)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	cfg.SecurityEnforcement = true
	if err := ticket.SaveConfig(h.repo, cfg); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	enabledOut, err := h.run("doctor")
	if err != nil {
		t.Fatalf("doctor failed after enabling enforcement: %v\n%s", err, enabledOut)
	}
	if !strings.Contains(enabledOut, "Security enforcement: enabled") {
		t.Fatalf("expected enabled enforcement note, got:\n%s", enabledOut)
	}
}
