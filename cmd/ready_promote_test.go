package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestReadyPromoteCommandPromotesPassingDraftLeaf(t *testing.T) {
	h := newFakeRepoHarness(t)

	s := local.New(h.repo)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-240",
		Seq:         240,
		Title:       "Draft leaf ready for promotion",
		State:       ticket.State("draft"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:harness-agent",
		Description: readyCheckDescription(),
		AC:          readyCheckAC(),
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	out, err := h.run("ready", "TKT-240", "--promote")
	if err != nil {
		t.Fatalf("expected draft promotion to succeed, err=%v output=%s", err, out)
	}
	if !strings.Contains(strings.ToLower(out), "promoted") {
		t.Fatalf("expected promotion output, got:\n%s", out)
	}

	stored, getErr := s.GetTicket(ctx, "TKT-240")
	if getErr != nil {
		t.Fatalf("get promoted ticket: %v", getErr)
	}
	if stored.State != ticket.State("ready") {
		t.Fatalf("expected promoted ticket state ready, got %s", stored.State)
	}
}

func TestReadyPromoteCommandRejectsContractFailuresWithoutMutatingDraft(t *testing.T) {
	h := newFakeRepoHarness(t)

	s := local.New(h.repo)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-241",
		Seq:         241,
		Title:       "Draft still missing ready contract fields",
		State:       ticket.State("draft"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:harness-agent",
		Description: "This draft is still too short and incomplete for runnable work.",
		AC: []ticket.AcceptanceCriterion{
			{Description: "Only one acceptance criterion"},
		},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	out, err := h.run("ready", "TKT-241", "--promote")
	if err == nil {
		t.Fatalf("expected invalid draft promotion to fail, output=%s", out)
	}
	for _, want := range []string{
		"ready_contract.description",
		"ready_contract.ac",
		"ready_contract.verification",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected failed promotion output to contain %q, got:\n%s", want, out)
		}
	}

	stored, getErr := s.GetTicket(ctx, "TKT-241")
	if getErr != nil {
		t.Fatalf("get invalid draft ticket: %v", getErr)
	}
	if stored.State != ticket.State("draft") {
		t.Fatalf("expected failed promotion to leave ticket in draft, got %s", stored.State)
	}
}

func TestReadyPromoteCommandRejectsNonLeafDraftTickets(t *testing.T) {
	h := newFakeRepoHarness(t)

	s := local.New(h.repo)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	for _, tk := range []*ticket.Ticket{
		{
			ID:          "TKT-242",
			Seq:         242,
			Title:       "Draft parent ticket",
			State:       ticket.State("draft"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "agent:harness-agent",
			Description: readyCheckDescription(),
			AC:          readyCheckAC(),
		},
		{
			ID:          "TKT-243",
			Seq:         243,
			Title:       "Child draft ticket",
			Parent:      "TKT-242",
			State:       ticket.State("draft"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "agent:harness-agent",
			Description: readyCheckDescription(),
			AC:          readyCheckAC(),
		},
	} {
		if err := s.CreateTicket(ctx, tk); err != nil {
			t.Fatalf("create %s: %v", tk.ID, err)
		}
	}

	out, err := h.run("ready", "TKT-242", "--promote")
	if err == nil {
		t.Fatalf("expected non-leaf promotion to fail, output=%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "leaf ticket") {
		t.Fatalf("expected non-leaf promotion rejection, got:\n%s", out)
	}

	stored, getErr := s.GetTicket(ctx, "TKT-242")
	if getErr != nil {
		t.Fatalf("get parent ticket: %v", getErr)
	}
	if stored.State != ticket.State("draft") {
		t.Fatalf("expected non-leaf promotion to leave ticket in draft, got %s", stored.State)
	}
}

func TestReadyPromoteCommandIsIdempotentForAlreadyReadyTickets(t *testing.T) {
	h := newFakeRepoHarness(t)

	s := local.New(h.repo)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-244",
		Seq:         244,
		Title:       "Already ready leaf ticket",
		State:       ticket.State("ready"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:harness-agent",
		Description: readyCheckDescription(),
		AC:          readyCheckAC(),
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	out, err := h.run("ready", "TKT-244", "--promote")
	if err != nil {
		t.Fatalf("expected already-ready promotion re-run to succeed, err=%v output=%s", err, out)
	}
	if !strings.Contains(strings.ToLower(out), "already ready") {
		t.Fatalf("expected idempotent already-ready message, got:\n%s", out)
	}
}

func TestReadyPromoteCommandRefreshesManifestAndIndex(t *testing.T) {
	h := newFakeRepoHarness(t)

	s := local.New(h.repo)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-245",
		Seq:         245,
		Title:       "Draft leaf visible after promotion",
		State:       ticket.State("draft"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:harness-agent",
		Description: readyCheckDescription(),
		AC:          readyCheckAC(),
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	if _, err := h.run("ready", "TKT-245", "--promote"); err != nil {
		t.Fatalf("expected promotion to succeed: %v", err)
	}

	manifestPath := filepath.Join(h.repo, ".docket", "manifest.json")
	manifestData, readErr := os.ReadFile(manifestPath)
	if readErr != nil {
		t.Fatalf("read manifest: %v", readErr)
	}
	var manifest local.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	entry, ok := manifest.Tickets["TKT-245"]
	if !ok {
		t.Fatalf("expected manifest entry for promoted ticket, got %#v", manifest.Tickets)
	}
	if entry.State != "ready" {
		t.Fatalf("expected manifest state ready after promotion, got %#v", entry)
	}

	tickets, err := s.ListTickets(ctx, store.Filter{States: []ticket.State{ticket.State("ready")}})
	if err != nil {
		t.Fatalf("list ready tickets: %v", err)
	}
	found := false
	for _, tk := range tickets {
		if tk.ID == "TKT-245" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected promoted ticket in ready index results, got %#v", tickets)
	}
}

func TestReadyPromoteCommandRejectsCoordinationTickets(t *testing.T) {
	h := newFakeRepoHarness(t)

	s := local.New(h.repo)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-246",
		Seq:         246,
		Title:       "Epic: coordination ticket",
		State:       ticket.State("draft"),
		Priority:    1,
		Labels:      []string{"epic"},
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:harness-agent",
		Description: readyCheckDescription(),
		AC:          readyCheckAC(),
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	out, err := h.run("ready", "TKT-246", "--promote")
	if err == nil {
		t.Fatalf("expected coordination-ticket promotion to fail, output=%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "coordination tickets") {
		t.Fatalf("expected coordination-ticket rejection, got:\n%s", out)
	}

	stored, getErr := s.GetTicket(ctx, "TKT-246")
	if getErr != nil {
		t.Fatalf("get coordination ticket: %v", getErr)
	}
	if stored.State != ticket.State("draft") {
		t.Fatalf("expected failed coordination-ticket promotion to leave ticket in draft, got %s", stored.State)
	}
}
