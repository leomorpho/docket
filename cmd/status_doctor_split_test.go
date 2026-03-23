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
	if !strings.Contains(statusOut, "--parallel") {
		t.Fatalf("status output should point to runtime parallel matrix follow-up, got:\n%s", statusOut)
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

func TestStatusParallelUsesConfiguredActiveRoleStates(t *testing.T) {
	h := newFakeRepoHarness(t)

	cfg := ticket.DefaultConfig()
	cfg.States = map[string]ticket.StateConfig{
		"queued":  {Label: "Queued", Open: true, Column: 0, Next: []string{"coding"}, Roles: []string{"intake"}, Startable: true, BlocksDependents: true},
		"coding":  {Label: "Coding", Open: true, Column: 1, Next: []string{"testing"}, Roles: []string{"active"}, BlocksDependents: true},
		"testing": {Label: "Testing", Open: true, Column: 2, Next: []string{"qa"}, Roles: []string{"active"}, BlocksDependents: true},
		"qa":      {Label: "QA", Open: true, Column: 3, Next: []string{"done"}, Roles: []string{"review"}, Reviewable: true, BlocksDependents: true},
		"done":    {Label: "Done", Open: false, Column: 4, Next: []string{}, Roles: []string{"completed"}, Terminal: true},
	}
	cfg.DefaultState = "queued"
	if err := ticket.SaveConfig(h.repo, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	s := local.New(h.repo)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-900",
		Seq:         900,
		Title:       "Coding task",
		State:       "coding",
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "Custom active role ticket",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
	}); err != nil {
		t.Fatalf("create coding ticket: %v", err)
	}
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-901",
		Seq:         901,
		Title:       "Testing task",
		State:       "testing",
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "Custom active role ticket",
		AC:          []ticket.AcceptanceCriterion{{Description: "B"}},
	}); err != nil {
		t.Fatalf("create testing ticket: %v", err)
	}

	out, err := h.run("status", "--parallel")
	if err != nil {
		t.Fatalf("status --parallel failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "TKT-900 <-> TKT-901") {
		t.Fatalf("expected custom active-role tickets in parallel matrix, got:\n%s", out)
	}
}
