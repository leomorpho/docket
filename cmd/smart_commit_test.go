package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func readyHandoffFixture() string {
	return strings.Join([]string{
		"**Current state:** implementation complete",
		"",
		"**Decisions made:** kept changes minimal and test-covered",
		"",
		"**Files touched:**",
		"- cmd/smart_commit.go",
		"",
		"**Remaining work:**",
		"- final review",
		"",
		"**AC status:**",
		"- complete",
	}, "\n")
}

func TestSmartCommit_GeneratesMessageAndTrailerWhenReady(t *testing.T) {
	h := newFakeRepoHarness(t)
	h.seedTicket("TKT-971", 971, ticket.State("running"), []ticket.AcceptanceCriterion{{Description: "ac", Done: true, Evidence: "verified"}, {Description: "second ac", Done: true, Evidence: "verified"}})

	s := local.New(h.repo)
	tkt, err := s.GetTicket(context.Background(), "TKT-971")
	if err != nil || tkt == nil {
		t.Fatalf("load ticket failed: %v", err)
	}
	tkt.Handoff = readyHandoffFixture()
	tkt.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	if err := s.UpdateTicket(context.Background(), tkt); err != nil {
		t.Fatalf("update ticket handoff failed: %v", err)
	}

	out, err := h.run("smart-commit", "TKT-971")
	if err != nil {
		t.Fatalf("smart-commit failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Smart commit for TKT-971: READY") {
		t.Fatalf("expected ready header, got:\n%s", out)
	}
	if !strings.Contains(out, "Ticket: TKT-971") {
		t.Fatalf("expected trailer in generated message, got:\n%s", out)
	}
	if !strings.Contains(out, "git commit -m") {
		t.Fatalf("expected suggested git commit command, got:\n%s", out)
	}
}

func TestSmartCommit_ValidateEnforcesTicketTrailer(t *testing.T) {
	h := newFakeRepoHarness(t)
	h.seedTicket("TKT-972", 972, ticket.State("running"), []ticket.AcceptanceCriterion{{Description: "ac", Done: true, Evidence: "verified"}, {Description: "second ac", Done: true, Evidence: "verified"}})

	s := local.New(h.repo)
	tkt, err := s.GetTicket(context.Background(), "TKT-972")
	if err != nil || tkt == nil {
		t.Fatalf("load ticket failed: %v", err)
	}
	tkt.Handoff = readyHandoffFixture()
	tkt.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	if err := s.UpdateTicket(context.Background(), tkt); err != nil {
		t.Fatalf("update ticket handoff failed: %v", err)
	}

	okOut, err := h.run("smart-commit", "TKT-972", "--validate", "feat: validate path\n\nTicket: TKT-972", "--format", "json")
	if err != nil {
		t.Fatalf("smart-commit validate should pass: %v\n%s", err, okOut)
	}
	var okPayload map[string]any
	if err := json.Unmarshal([]byte(okOut), &okPayload); err != nil {
		t.Fatalf("unmarshal validate json failed: %v\n%s", err, okOut)
	}
	if okPayload["validated"] != true {
		t.Fatalf("expected validated=true, got %#v", okPayload)
	}

	badOut, err := h.run("smart-commit", "TKT-972", "--validate", "feat: wrong trailer\n\nTicket: TKT-999")
	if err == nil {
		t.Fatalf("expected trailer mismatch to fail, output=%s", badOut)
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("expected trailer mismatch error, got: %v", err)
	}
}

func TestSmartCommit_FailsWhenTicketNotReady(t *testing.T) {
	h := newFakeRepoHarness(t)
	h.seedTicket("TKT-973", 973, ticket.State("running"), []ticket.AcceptanceCriterion{{Description: "ac", Done: false}, {Description: "second ac", Done: false}})

	out, err := h.run("smart-commit", "TKT-973")
	if err == nil {
		t.Fatalf("expected not-ready ticket to fail, output=%s", out)
	}
	if !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("expected not-ready error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "ac_complete") {
		t.Fatalf("expected failed check id in error, got: %v", err)
	}
}
