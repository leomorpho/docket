package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	docketgit "github.com/leomorpho/docket/internal/git"
	"github.com/leomorpho/docket/internal/lifecycle"
	"github.com/leomorpho/docket/internal/proof"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func writeShowProofPNG(t *testing.T, path string) {
	t.Helper()
	data := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde,
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture dir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func TestShowCmd(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	// 0. Setup
	s := local.New(tmpDir)
	ticket.SaveConfig(tmpDir, ticket.DefaultConfig())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	tick := &ticket.Ticket{
		ID:          "TKT-001",
		Title:       "Test Ticket",
		State:       ticket.State("todo"),
		Priority:    1,
		Description: "Desc here",
		AC:          []ticket.AcceptanceCriterion{{Description: "AC1", Done: true}},
		Plan:        []ticket.PlanStep{{Description: "Step 1", Status: "pending"}},
		Comments:    []ticket.Comment{{Author: "me", Body: "hello"}},
		Handoff:     "some handoff",
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
	}
	s.CreateTicket(ctx, tick)
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-002",
		Title:       "Child Ticket",
		Parent:      "TKT-001",
		State:       ticket.State("todo"),
		Priority:    2,
		Description: "Child desc",
		AC: []ticket.AcceptanceCriterion{
			{Description: "AC child 1", Done: true},
			{Description: "AC child 2", Done: false},
		},
		CreatedAt: now.Add(time.Minute),
		UpdatedAt: now.Add(time.Minute),
		CreatedBy: "me",
	}); err != nil {
		t.Fatalf("create child ticket failed: %v", err)
	}

	// 1. Human format
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"show", "TKT-001"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show failed: %v", err)
	}
	if !strings.Contains(b.String(), "TKT-001 · todo") {
		t.Errorf("expected header, got:\n%s", b.String())
	}
	if !strings.Contains(b.String(), "Test Ticket") {
		t.Errorf("expected title, got:\n%s", b.String())
	}
	if !strings.Contains(b.String(), "Descendant AC: 1/2 done across 1 descendant tickets") {
		t.Errorf("expected descendant AC aggregation in show output, got:\n%s", b.String())
	}

	// 2. MD format
	b.Reset()
	format = "md"
	rootCmd.SetArgs([]string{"show", "TKT-001", "--format", "md"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show md failed: %v", err)
	}
	if !strings.HasPrefix(b.String(), "---") {
		t.Errorf("expected raw MD starting with ---, got:\n%s", b.String())
	}

	// 3. JSON format
	b.Reset()
	format = "json"
	rootCmd.SetArgs([]string{"show", "TKT-001", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show json failed: %v", err)
	}
	var res map[string]interface{}
	if err := json.Unmarshal(b.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if res["id"] != "TKT-001" {
		t.Errorf("expected ID TKT-001, got: %v", res["id"])
	}

	// 4. Context format
	b.Reset()
	format = "context"
	rootCmd.SetArgs([]string{"show", "TKT-001", "--format", "context"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show context failed: %v", err)
	}
	if !strings.Contains(b.String(), "TICKET: TKT-001") {
		t.Errorf("expected compact context output, got:\n%s", b.String())
	}

	// 5. Not found
	b.Reset()
	format = "human"
	rootCmd.SetArgs([]string{"show", "TKT-999"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent ticket, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestShowCmd_AcceptsBracketedTicketIDFromContextList(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "Bracketed lookup fixture",
		State:       ticket.State("todo"),
		Priority:    1,
		Description: "desc",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"show", "[TKT-001]"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show with bracketed id failed: %v", err)
	}
	if !strings.Contains(out.String(), "TKT-001 · todo") {
		t.Fatalf("expected ticket details for canonical id, got:\n%s", out.String())
	}
}

func TestShowCmd_UsesSharedRepoRootWhenInvokedFromWorktree(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	runGitSession(t, tmpDir, "init")
	runGitSession(t, tmpDir, "config", "user.email", "test@example.com")
	runGitSession(t, tmpDir, "config", "user.name", "Test User")

	s := local.New(tmpDir)
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config failed: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-111",
		Seq:         111,
		Title:       "Canonical root ticket",
		State:       ticket.State("todo"),
		Priority:    1,
		Description: "desc",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
	}); err != nil {
		t.Fatalf("create ticket failed: %v", err)
	}
	runGitSession(t, tmpDir, "add", ".")
	runGitSession(t, tmpDir, "commit", "-m", "chore: seed ticket")

	worktreePath := filepath.Join(tmpDir, "wt", "TKT-111")
	if err := docketgit.CreateWorktree(tmpDir, "TKT-111", "docket/TKT-111", worktreePath); err != nil {
		t.Fatalf("create worktree failed: %v", err)
	}

	worktreeTicketPath := filepath.Join(worktreePath, ".docket", "tickets", "TKT-111.md")
	raw, err := os.ReadFile(worktreeTicketPath)
	if err != nil {
		t.Fatalf("read worktree ticket failed: %v", err)
	}
	stale := strings.Replace(string(raw), "state: todo", "state: in-progress", 1)
	if stale == string(raw) {
		t.Fatal("expected worktree ticket state line to be rewritten")
	}
	if err := os.WriteFile(worktreeTicketPath, []byte(stale), 0o644); err != nil {
		t.Fatalf("write stale worktree ticket failed: %v", err)
	}

	oldRepo := repo
	repo = worktreePath
	t.Cleanup(func() { repo = oldRepo })

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"show", "TKT-111"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show from worktree failed: %v", err)
	}
	if !strings.Contains(out.String(), "TKT-111 · todo") {
		t.Fatalf("expected show to read canonical root ticket state, got:\n%s", out.String())
	}
	if strings.Contains(out.String(), "TKT-111 · in-progress") {
		t.Fatalf("expected stale worktree state to be ignored, got:\n%s", out.String())
	}
}

func TestShowCmd_ProofMetadataAppearsWhenPresent(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	ticket.SaveConfig(tmpDir, ticket.DefaultConfig())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	tick := &ticket.Ticket{
		ID:          "TKT-001",
		Title:       "Proof Ticket",
		State:       ticket.State("todo"),
		Priority:    1,
		Description: "Desc here",
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		AC:          []ticket.AcceptanceCriterion{{Description: "AC1", Done: true}},
	}
	if err := s.CreateTicket(ctx, tick); err != nil {
		t.Fatalf("create ticket failed: %v", err)
	}

	proofRel := filepath.Join("fixtures", "proof.png")
	proofAbs := filepath.Join(tmpDir, proofRel)
	writeShowProofPNG(t, proofAbs)
	if _, err := s.AddProof(ctx, proof.AddInput{
		TicketID:   "TKT-001",
		SourcePath: proofRel,
		ProofTitle: "Before",
		Note:       "baseline proof",
		AddedAt:    now.Add(time.Minute).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("add proof failed: %v", err)
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"show", "TKT-001"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show human failed: %v", err)
	}
	if !strings.Contains(b.String(), "Proofs (1)") || !strings.Contains(b.String(), "baseline proof") {
		t.Fatalf("expected proof summary in human output, got:\n%s", b.String())
	}

	b.Reset()
	format = "json"
	rootCmd.SetArgs([]string{"show", "TKT-001", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show json failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(b.Bytes(), &payload); err != nil {
		t.Fatalf("parse show json failed: %v", err)
	}
	proofsAny, ok := payload["proofs"].([]any)
	if !ok || len(proofsAny) != 1 {
		t.Fatalf("expected one proof entry in json output, got %+v", payload["proofs"])
	}

	b.Reset()
	format = "context"
	rootCmd.SetArgs([]string{"show", "TKT-001", "--format", "context"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show context failed: %v", err)
	}
	if !strings.Contains(b.String(), "PROOFS:") {
		t.Fatalf("expected PROOFS summary in context output, got:\n%s", b.String())
	}
}

func TestShowCmd_JSONIncludesTransitionHistory(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"

	s := local.New(tmpDir)
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config failed: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-001",
		Title:       "Transition Ticket",
		State:       ticket.State("in-review"),
		Priority:    1,
		Description: "desc",
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
	}); err != nil {
		t.Fatalf("create ticket failed: %v", err)
	}

	if err := lifecycle.Append(tmpDir, lifecycle.Event{
		Version:   lifecycle.SchemaVersionV1,
		Type:      lifecycle.EventStateTransition,
		EmittedAt: now.Add(time.Minute).Format(time.RFC3339Nano),
		Payload: map[string]any{
			"command":            "update",
			"ticket_id":          "TKT-001",
			"actor":              "human:test",
			"from_state":         "todo",
			"to_state":           "in-review",
			"reason":             "update --state in-review",
			"checks":             []any{"state_transition_validated", "managed_run_commit_linkage"},
			"managed_run":        true,
			"run_branch":         "docket/TKT-001",
		},
	}); err != nil {
		t.Fatalf("append transition event failed: %v", err)
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"show", "TKT-001", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show json failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(b.Bytes(), &payload); err != nil {
		t.Fatalf("parse show json failed: %v", err)
	}
	historyAny, ok := payload["transition_history"].([]any)
	if !ok || len(historyAny) != 1 {
		t.Fatalf("expected one transition history entry, got %#v", payload["transition_history"])
	}
	first, ok := historyAny[0].(map[string]any)
	if !ok {
		t.Fatalf("expected transition object, got %#v", historyAny[0])
	}
	if first["from_state"] != "todo" || first["to_state"] != "in-review" {
		t.Fatalf("unexpected transition states: %#v", first)
	}
	if first["actor"] != "human:test" {
		t.Fatalf("unexpected transition actor: %#v", first)
	}
}

func TestShowCmd_DiscoveryHintShownForHumanButNotJSONOrMD(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config failed: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "Hint fixture",
		State:       ticket.State("todo"),
		Priority:    1,
		Description: "desc",
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
	}); err != nil {
		t.Fatalf("create ticket failed: %v", err)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"show", "TKT-001"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show human failed: %v", err)
	}
	if !strings.Contains(out.String(), "Skill hint: use `docket skill invoke <skill-id>`") {
		t.Fatalf("expected short skill hint in show human output, got:\n%s", out.String())
	}

	out.Reset()
	rootCmd.SetArgs([]string{"show", "TKT-001", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show json failed: %v", err)
	}
	if strings.Contains(out.String(), "Skill hint:") {
		t.Fatalf("expected no skill hint in show json output, got:\n%s", out.String())
	}

	out.Reset()
	rootCmd.SetArgs([]string{"show", "TKT-001", "--format", "md"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show md failed: %v", err)
	}
	if strings.Contains(out.String(), "Skill hint:") {
		t.Fatalf("expected no skill hint in show md output, got:\n%s", out.String())
	}
}
