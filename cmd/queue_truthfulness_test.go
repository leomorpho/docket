package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/agentrun"
	selectorpkg "github.com/leomorpho/docket/internal/agentrun/selector"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

type queueTruthScenario struct {
	name       string
	seed       func(t *testing.T, h *fakeRepoHarness)
	wantReason string
}

func TestQueueHealthSurfacesAgreeWhenNoRunnableWorkExists(t *testing.T) {
	scenarios := []queueTruthScenario{
		{
			name: "zero ready tickets",
			seed: func(t *testing.T, h *fakeRepoHarness) {
				queueTruthSeedTickets(t, h.repo, []*ticket.Ticket{
					{
						ID:          "TKT-100",
						Seq:         100,
						Title:       "Draft ticket only",
						State:       ticket.State("draft"),
						Priority:    1,
						Description: "Draft ticket that keeps the queue empty for queue-truthfulness coverage.",
						AC:          []ticket.AcceptanceCriterion{{Description: "Draft ticket remains present"}},
					},
				})
			},
			wantReason: "no actionable tickets in startable states",
		},
		{
			name: "ready ticket fails ready contract",
			seed: func(t *testing.T, h *fakeRepoHarness) {
				queueTruthSeedTickets(t, h.repo, []*ticket.Ticket{
					{
						ID:          "TKT-200",
						Seq:         200,
						Title:       "Ungroomed ready ticket",
						State:       ticket.State("ready"),
						Priority:    1,
						Description: "Short description without runnable sections.",
						AC:          []ticket.AcceptanceCriterion{{Description: "Only one AC"}},
					},
				})
			},
			wantReason: "ready contract is incomplete",
		},
		{
			name: "blocked ready ticket",
			seed: func(t *testing.T, h *fakeRepoHarness) {
				queueTruthSeedTickets(t, h.repo, []*ticket.Ticket{
					{
						ID:          "TKT-300",
						Seq:         300,
						Title:       "Active blocker",
						State:       ticket.State("running"),
						Priority:    1,
						Description: updateRunnableDescription(),
						AC:          updateRunnableAC(),
					},
					{
						ID:          "TKT-301",
						Seq:         301,
						Title:       "Blocked ready ticket",
						State:       ticket.State("ready"),
						Priority:    2,
						BlockedBy:   []string{"TKT-300"},
						Description: updateRunnableDescription(),
						AC:          updateRunnableAC(),
					},
				})
			},
			wantReason: "Top unresolved blockers: TKT-300 x1",
		},
	}

	for _, tc := range scenarios {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			startHuman := queueTruthStartHuman(t, tc.seed)
			if !strings.Contains(startHuman, tc.wantReason) {
				t.Fatalf("start human output must explain the empty runnable queue, got:\n%s", startHuman)
			}

			startJSON := queueTruthStartJSON(t, tc.seed)
			if startJSON["no_workable_ticket"] != true {
				t.Fatalf("expected start json to report no workable ticket, got %#v", startJSON)
			}
			message, _ := startJSON["message"].(string)
			if !strings.Contains(message, tc.wantReason) {
				t.Fatalf("start json message must explain the empty runnable queue, got %q", message)
			}

			selection := queueTruthSelection(t, tc.seed)
			if selection.Found {
				t.Fatalf("selector must report no runnable ticket, got %#v", selection)
			}
			if !strings.Contains(selection.Reason, tc.wantReason) {
				t.Fatalf("selector reason must explain the empty runnable queue, got %q", selection.Reason)
			}

			doctorHuman := queueTruthDoctorHuman(t, tc.seed)
			if !strings.Contains(doctorHuman, "queue_invariant") || !strings.Contains(doctorHuman, tc.wantReason) {
				t.Fatalf("doctor human output must agree with start/selector about the empty runnable queue, got:\n%s", doctorHuman)
			}

			doctorJSON := queueTruthDoctorJSON(t, tc.seed)
			if statusByName(doctorJSON.Checks, "queue_invariant") != "FAIL" {
				t.Fatalf("doctor json must fail queue_invariant when no ticket is runnable, got %#v", doctorJSON.Checks)
			}
			if !strings.Contains(queueTruthDoctorDetail(doctorJSON.Checks), tc.wantReason) {
				t.Fatalf("doctor json detail must explain the empty runnable queue, got %#v", doctorJSON.Checks)
			}

			statusHuman := queueTruthStatusHuman(t, tc.seed)
			if !strings.Contains(statusHuman, tc.wantReason) {
				t.Fatalf("status output must agree with the empty runnable queue diagnosis, got:\n%s", statusHuman)
			}
		})
	}
}

func TestQueueHealthSurfacesAgreeWhenOneRunnableReadyLeafExists(t *testing.T) {
	const wantTicketID = "TKT-401"
	seed := func(t *testing.T, h *fakeRepoHarness) {
		queueTruthSeedTickets(t, h.repo, []*ticket.Ticket{
			{
				ID:          wantTicketID,
				Seq:         401,
				Title:       "Runnable ready leaf",
				State:       ticket.State("ready"),
				Priority:    1,
				Description: updateRunnableDescription(),
				AC:          updateRunnableAC(),
			},
		})
	}

	startHuman := queueTruthStartHuman(t, seed)
	if !strings.Contains(startHuman, "You have started working on ticket: "+wantTicketID) {
		t.Fatalf("start human output must point to the runnable ticket, got:\n%s", startHuman)
	}

	startJSON := queueTruthStartJSON(t, seed)
	if startJSON["no_workable_ticket"] == true {
		t.Fatalf("expected start json to return a runnable ticket, got %#v", startJSON)
	}
	startTicket, ok := startJSON["ticket"].(map[string]any)
	if !ok || startTicket["id"] != wantTicketID {
		t.Fatalf("start json must identify the runnable ticket, got %#v", startJSON)
	}

	selection := queueTruthSelection(t, seed)
	if !selection.Found || selection.TicketID != wantTicketID {
		t.Fatalf("selector must return the runnable ticket, got %#v", selection)
	}

	doctorHuman := queueTruthDoctorHuman(t, seed)
	if !strings.Contains(doctorHuman, "PASS queue_invariant") {
		t.Fatalf("doctor human output must acknowledge runnable work, got:\n%s", doctorHuman)
	}

	doctorJSON := queueTruthDoctorJSON(t, seed)
	if statusByName(doctorJSON.Checks, "queue_invariant") != "PASS" {
		t.Fatalf("doctor json must pass queue_invariant when runnable work exists, got %#v", doctorJSON.Checks)
	}

	statusHuman := queueTruthStatusHuman(t, seed)
	if !strings.Contains(statusHuman, wantTicketID) {
		t.Fatalf("status output must identify the same runnable ticket as start/selector, got:\n%s", statusHuman)
	}
}

func queueTruthSeedTickets(t *testing.T, repoRoot string, tickets []*ticket.Ticket) {
	t.Helper()

	s := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	for i, tk := range tickets {
		item := *tk
		if item.CreatedAt.IsZero() {
			item.CreatedAt = now.Add(time.Duration(i) * time.Minute)
		}
		if item.UpdatedAt.IsZero() {
			item.UpdatedAt = item.CreatedAt
		}
		if item.CreatedBy == "" {
			item.CreatedBy = "agent:test"
		}
		if err := s.CreateTicket(context.Background(), &item); err != nil {
			t.Fatalf("seed %s failed: %v", item.ID, err)
		}
	}
}

func queueTruthHarness(t *testing.T, seed func(t *testing.T, h *fakeRepoHarness)) *fakeRepoHarness {
	t.Helper()

	h := newFakeRepoHarness(t)
	seed(t, h)
	return h
}

func queueTruthStartHuman(t *testing.T, seed func(t *testing.T, h *fakeRepoHarness)) string {
	t.Helper()

	h := queueTruthHarness(t, seed)
	out, err := h.run("start")
	if err != nil {
		t.Fatalf("start failed: %v\n%s", err, out)
	}
	return out
}

func queueTruthStartJSON(t *testing.T, seed func(t *testing.T, h *fakeRepoHarness)) map[string]any {
	t.Helper()

	h := queueTruthHarness(t, seed)
	out, err := h.run("start", "--format", "json")
	if err != nil {
		t.Fatalf("start --format json failed: %v\n%s", err, out)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal start json failed: %v\n%s", err, out)
	}
	return payload
}

func queueTruthDoctorHuman(t *testing.T, seed func(t *testing.T, h *fakeRepoHarness)) string {
	t.Helper()

	h := queueTruthHarness(t, seed)
	out, err := h.run("doctor")
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, out)
	}
	return out
}

func queueTruthDoctorJSON(t *testing.T, seed func(t *testing.T, h *fakeRepoHarness)) doctorReport {
	t.Helper()

	h := queueTruthHarness(t, seed)
	out, err := h.run("doctor", "--format", "json")
	if err != nil {
		t.Fatalf("doctor --format json failed: %v\n%s", err, out)
	}
	var payload doctorReport
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal doctor json failed: %v\n%s", err, out)
	}
	return payload
}

func queueTruthStatusHuman(t *testing.T, seed func(t *testing.T, h *fakeRepoHarness)) string {
	t.Helper()

	h := queueTruthHarness(t, seed)
	out, err := h.run("status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, out)
	}
	return out
}

func queueTruthSelection(t *testing.T, seed func(t *testing.T, h *fakeRepoHarness)) agentrun.Selection {
	t.Helper()

	h := queueTruthHarness(t, seed)
	selection, err := selectorpkg.New(selectorpkg.Dependencies{Store: local.New(h.repo)}).Next(context.Background())
	if err != nil {
		t.Fatalf("selector next failed: %v", err)
	}
	return selection
}

func queueTruthDoctorDetail(checks []doctorCheck) string {
	for _, check := range checks {
		if check.Name == "queue_invariant" {
			return check.Detail
		}
	}
	return ""
}
