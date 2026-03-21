package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestHealthCmdConnectedGraphPasses(t *testing.T) {
	tmpDir := t.TempDir()
	oldRepo := repo
	oldFormat := format
	repo = tmpDir
	format = "human"
	defer func() {
		repo = oldRepo
		format = oldFormat
	}()

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	createHealthTicket(t, s, &ticket.Ticket{
		ID: "TKT-001", Seq: 1, Title: "Root", State: "backlog", Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "human:test",
		Description: strings.Repeat("connected graph ", 5),
		AC:          []ticket.AcceptanceCriterion{{Description: "ac1"}, {Description: "ac2"}},
	})
	createHealthTicket(t, s, &ticket.Ticket{
		ID: "TKT-002", Seq: 2, Title: "Child", State: "backlog", Priority: 1, Parent: "TKT-001",
		CreatedAt: now, UpdatedAt: now, CreatedBy: "human:test",
		Description: strings.Repeat("connected graph ", 5),
		AC:          []ticket.AcceptanceCriterion{{Description: "ac1"}, {Description: "ac2"}},
	})
	createHealthTicket(t, s, &ticket.Ticket{
		ID: "TKT-003", Seq: 3, Title: "Peer", State: "backlog", Priority: 1, BlockedBy: []string{"TKT-002"},
		CreatedAt: now, UpdatedAt: now, CreatedBy: "human:test",
		Description: strings.Repeat("connected graph ", 5),
		AC:          []ticket.AcceptanceCriterion{{Description: "ac1"}, {Description: "ac2"}},
	})

	report, err := buildHealthReport(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("build health report: %v", err)
	}
	if !report.OK {
		t.Fatalf("expected healthy report, got %+v", report)
	}

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"health"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("health failed: %v\noutput:\n%s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "Ticket graph healthy") {
		t.Fatalf("expected healthy output, got:\n%s", buf.String())
	}
}

func TestHealthCmdDisconnectedGraphFails(t *testing.T) {
	tmpDir := t.TempDir()
	oldRepo := repo
	oldFormat := format
	repo = tmpDir
	format = "human"
	defer func() {
		repo = oldRepo
		format = oldFormat
	}()

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	createHealthTicket(t, s, &ticket.Ticket{
		ID: "TKT-001", Seq: 1, Title: "Root A", State: "backlog", Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "human:test",
		Description: strings.Repeat("disconnected graph ", 5),
		AC:          []ticket.AcceptanceCriterion{{Description: "ac1"}, {Description: "ac2"}},
	})
	createHealthTicket(t, s, &ticket.Ticket{
		ID: "TKT-002", Seq: 2, Title: "Child A", State: "backlog", Priority: 1, Parent: "TKT-001",
		CreatedAt: now, UpdatedAt: now, CreatedBy: "human:test",
		Description: strings.Repeat("disconnected graph ", 5),
		AC:          []ticket.AcceptanceCriterion{{Description: "ac1"}, {Description: "ac2"}},
	})
	createHealthTicket(t, s, &ticket.Ticket{
		ID: "TKT-003", Seq: 3, Title: "Root B", State: "backlog", Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "human:test",
		Description: strings.Repeat("disconnected graph ", 5),
		AC:          []ticket.AcceptanceCriterion{{Description: "ac1"}, {Description: "ac2"}},
	})

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"health"})
	err := rootCmd.Execute()
	if !errors.Is(err, errHealthFindings) {
		t.Fatalf("expected health findings error, got %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "graph_disconnected") || !strings.Contains(out, "TKT-003") {
		t.Fatalf("expected disconnected graph output, got:\n%s", out)
	}
}

func TestHealthCmdJSONIncludesMissingRelationEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	oldRepo := repo
	oldFormat := format
	repo = tmpDir
	format = "json"
	defer func() {
		repo = oldRepo
		format = oldFormat
	}()

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	createHealthTicket(t, s, &ticket.Ticket{
		ID: "TKT-001", Seq: 1, Title: "Root", State: "backlog", Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "human:test",
		Description: strings.Repeat("relation graph ", 5),
		AC:          []ticket.AcceptanceCriterion{{Description: "ac1"}, {Description: "ac2"}},
	})
	if err := upsertRelation(tmpDir, relationEntry{From: "TKT-001", To: "TKT-999", Relation: "depends-on"}); err != nil {
		t.Fatalf("upsert relation: %v", err)
	}

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"health", "--format", "json"})
	err := rootCmd.Execute()
	if !errors.Is(err, errHealthFindings) {
		t.Fatalf("expected health findings error, got %v", err)
	}

	var report healthReport
	if json.Unmarshal(buf.Bytes(), &report) != nil {
		t.Fatalf("failed to parse JSON output: %s", buf.String())
	}
	if report.OK {
		t.Fatalf("expected report to fail, got ok=true")
	}
	found := false
	for _, issue := range report.Issues {
		if issue.Code == "missing_relation_endpoint" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected missing_relation_endpoint in report, got %+v", report.Issues)
	}
}

func TestHealthCmdEndToEndCLIFlow(t *testing.T) {
	tmpDir := t.TempDir()
	oldRepo := repo
	oldFormat := format
	repo = tmpDir
	format = "human"
	defer func() {
		repo = oldRepo
		format = oldFormat
	}()

	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(errOut)

	run := func(args ...string) error {
		out.Reset()
		errOut.Reset()
		rootCmd.SetArgs(args)
		return rootCmd.Execute()
	}

	desc := "Likely paths: cmd/health.go. Verify commands: go test ./cmd -run TestHealthCmdEndToEndCLIFlow -count=1. Out of scope: unrelated command surfaces."

	if err := run("create", "--title", "Root", "--priority", "1", "--desc", desc, "--ac", "root connected"); err != nil {
		t.Fatalf("create root: %v", err)
	}
	if err := run("create", "--title", "Child", "--priority", "1", "--parent", "TKT-001", "--desc", desc, "--ac", "child connected"); err != nil {
		t.Fatalf("create child: %v", err)
	}
	if err := run("health"); err != nil {
		t.Fatalf("health should pass after parent wiring: %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errOut.String())
	}

	err := run("create", "--title", "Detached", "--priority", "1", "--desc", desc, "--ac", "detached connected")
	if err == nil {
		t.Fatalf("expected disconnected create to fail")
	}
	if !strings.Contains(err.Error(), "must connect to the existing ticket graph") {
		t.Fatalf("expected connectivity failure, got %v", err)
	}

	if err := run("create", "--title", "Attached", "--priority", "1", "--blocked-by", "TKT-002", "--desc", desc, "--ac", "attached connected"); err != nil {
		t.Fatalf("create attached: %v", err)
	}
	if err := run("health"); err != nil {
		t.Fatalf("health should pass after blocked-by wiring: %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errOut.String())
	}
}

func TestHealthCmdLinkConnectsDisconnectedComponents(t *testing.T) {
	tmpDir := t.TempDir()
	oldRepo := repo
	oldFormat := format
	repo = tmpDir
	format = "human"
	defer func() {
		repo = oldRepo
		format = oldFormat
	}()

	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	createHealthTicket(t, s, &ticket.Ticket{
		ID: "TKT-010", Seq: 10, Title: "A", State: "backlog", Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "human:test",
		Description: strings.Repeat("link connect ", 5),
		AC:          []ticket.AcceptanceCriterion{{Description: "ac1"}, {Description: "ac2"}},
	})
	createHealthTicket(t, s, &ticket.Ticket{
		ID: "TKT-011", Seq: 11, Title: "B", State: "backlog", Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "human:test",
		Description: strings.Repeat("link connect ", 5),
		AC:          []ticket.AcceptanceCriterion{{Description: "ac1"}, {Description: "ac2"}},
	})

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(new(bytes.Buffer))

	rootCmd.SetArgs([]string{"health"})
	if err := rootCmd.Execute(); !errors.Is(err, errHealthFindings) {
		t.Fatalf("expected disconnected graph before link, got %v", err)
	}

	out.Reset()
	rootCmd.SetArgs([]string{"link", "TKT-010", "TKT-011", "--relation", "depends-on"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("link should connect components: %v", err)
	}

	out.Reset()
	rootCmd.SetArgs([]string{"health"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("health should pass after link: %v", err)
	}
}

func TestUpdateRejectsDisconnectingMutationAndRollsBack(t *testing.T) {
	tmpDir := t.TempDir()
	oldRepo := repo
	oldFormat := format
	repo = tmpDir
	format = "human"
	defer func() {
		repo = oldRepo
		format = oldFormat
	}()

	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	createHealthTicket(t, s, &ticket.Ticket{
		ID: "TKT-020", Seq: 20, Title: "Root", State: "backlog", Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "human:test",
		Description: strings.Repeat("update rollback ", 5),
		AC:          []ticket.AcceptanceCriterion{{Description: "ac1"}, {Description: "ac2"}},
	})
	createHealthTicket(t, s, &ticket.Ticket{
		ID: "TKT-021", Seq: 21, Title: "Child", State: "backlog", Priority: 1, Parent: "TKT-020",
		CreatedAt: now, UpdatedAt: now, CreatedBy: "human:test",
		Description: strings.Repeat("update rollback ", 5),
		AC:          []ticket.AcceptanceCriterion{{Description: "ac1"}, {Description: "ac2"}},
	})

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(new(bytes.Buffer))

	rootCmd.SetArgs([]string{"update", "TKT-021", "--parent", "none"})
	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "mutation disconnected the ticket graph") {
		t.Fatalf("expected disconnecting update to fail, got %v", err)
	}

	ticketAfter, getErr := s.GetTicket(context.Background(), "TKT-021")
	if getErr != nil {
		t.Fatalf("get ticket after failed update: %v", getErr)
	}
	if ticketAfter.Parent != "TKT-020" {
		t.Fatalf("expected parent rollback to preserve TKT-020, got %q", ticketAfter.Parent)
	}
}

func createHealthTicket(t *testing.T, s *local.Store, tk *ticket.Ticket) {
	t.Helper()
	if err := s.CreateTicket(context.Background(), tk); err != nil {
		t.Fatalf("create ticket %s: %v", tk.ID, err)
	}
}
