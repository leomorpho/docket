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

	"github.com/leomorpho/docket/internal/security"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestUpdateCmd(t *testing.T) {
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
		Title:       "Original Title",
		State:       ticket.State("backlog"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{}},
	}
	s.CreateTicket(ctx, tick)

	// 1. Update state (backlog -> todo)
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)

	updateState = "todo"
	rootCmd.SetArgs([]string{"update", "TKT-001", "--state", "todo"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("update state failed: %v", err)
	}
	if !strings.Contains(b.String(), "state backlog → todo") {
		t.Errorf("expected state transition message, got: %s", b.String())
	}

	updated, _ := s.GetTicket(ctx, "TKT-001")
	if updated.State != ticket.State("todo") {
		t.Errorf("expected state todo, got %s", updated.State)
	}

	// 2. Invalid transition (todo -> done)
	updateState = "done"
	rootCmd.SetArgs([]string{"update", "TKT-001", "--state", "done"})
	if err := rootCmd.Execute(); err == nil {
		t.Error("expected error for invalid transition todo -> done, got nil")
	}

	// 3. Labels and Blockers
	b.Reset()
	// Reset state flag manually since we use global variables and cmd.Flags().Changed persists
	updateState = ""
	updateAddLabels = []string{"feat"}
	updateBlockedBy = []string{"TKT-002"}
	rootCmd.SetArgs([]string{"update", "TKT-001", "--add-label", "feat", "--blocked-by", "TKT-002"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("update labels/blockers failed: %v", err)
	}
	updated, _ = s.GetTicket(ctx, "TKT-001")
	if len(updated.Labels) != 1 || updated.Labels[0] != "feat" {
		t.Errorf("Labels mismatch: %v", updated.Labels)
	}
	if len(updated.BlockedBy) != 1 || updated.BlockedBy[0] != "TKT-002" {
		t.Errorf("BlockedBy mismatch: %v", updated.BlockedBy)
	}

	// 4. JSON output
	format = "json"
	b.Reset()
	rootCmd.SetArgs([]string{"update", "TKT-001", "--priority", "2"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("JSON update failed: %v", err)
	}
	var res map[string]interface{}
	if err := json.Unmarshal(b.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if res["priority"].(float64) != 2 {
		t.Errorf("expected priority 2 in JSON, got: %v", res["priority"])
	}
}

func TestUpdateCmd_Handoff(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	tick := &ticket.Ticket{
		ID:          "TKT-001",
		Title:       "Needs Handoff",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
	}
	if err := s.CreateTicket(ctx, tick); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	handoff := "**Current state:**\nDone.\n\n**Decisions made:**\nNone.\n\n**Files touched:**\n- x\n\n**Remaining work:**\n- y\n\n**AC status:**\n- z"

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"update", "TKT-001", "--handoff", handoff})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("update handoff failed: %v", err)
	}

	updated, err := s.GetTicket(ctx, "TKT-001")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if updated.Handoff != handoff {
		t.Fatalf("handoff mismatch:\n%s", updated.Handoff)
	}
}

func TestUpdateCmd_HandoffFromStdin(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	tick := &ticket.Ticket{
		ID:          "TKT-001",
		Title:       "Needs Stdin Handoff",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
	}
	if err := s.CreateTicket(ctx, tick); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	handoff := "stdin handoff body\nwith two lines"
	origStdin := os.Stdin
	defer func() { os.Stdin = origStdin }()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	if _, err := w.WriteString(handoff); err != nil {
		t.Fatalf("write pipe failed: %v", err)
	}
	w.Close()
	os.Stdin = r

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-001", "--handoff", "-"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("update handoff from stdin failed: %v", err)
	}

	updated, err := s.GetTicket(ctx, "TKT-001")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if updated.Handoff != handoff {
		t.Fatalf("handoff mismatch:\n%s", updated.Handoff)
	}
}

func TestUpdateCmd_StaleRequiresCascadeForOpenChildren(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	cfg := ticket.DefaultConfig()
	cfg.States["stale"] = ticket.StateConfig{
		Label:  "Stale",
		Open:   false,
		Column: 6,
		Next:   []string{"todo", "archived"},
	}
	todo := cfg.States["todo"]
	todo.Next = append(todo.Next, "stale")
	cfg.States["todo"] = todo
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	s := local.New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "Parent",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
	}); err != nil {
		t.Fatalf("CreateTicket parent failed: %v", err)
	}
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-002",
		Seq:         2,
		Title:       "Child",
		Parent:      "TKT-001",
		State:       ticket.State("todo"),
		Priority:    2,
		CreatedAt:   now.Add(time.Minute),
		UpdatedAt:   now.Add(time.Minute),
		CreatedBy:   "me",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
	}); err != nil {
		t.Fatalf("CreateTicket child failed: %v", err)
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"update", "TKT-001", "--state", "stale"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected stale transition error without --cascade")
	}

	b.Reset()
	rootCmd.SetArgs([]string{"update", "TKT-001", "--state", "stale", "--cascade"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("stale transition with cascade failed: %v", err)
	}

	parent, _ := s.GetTicket(ctx, "TKT-001")
	child, _ := s.GetTicket(ctx, "TKT-002")
	if parent.State != ticket.State("stale") {
		t.Fatalf("parent state = %s, want stale", parent.State)
	}
	if child.State != ticket.State("stale") {
		t.Fatalf("child state = %s, want stale", child.State)
	}
}

func TestUpdateCmd_PrivilegedDoneRequiresSecureSurface(t *testing.T) {
	tmpDir := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	docketHome = ""
	repo = tmpDir
	format = "human"

	cfg := ticket.DefaultConfig()
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-186",
		Seq:         186,
		Title:       "Privileged transition",
		State:       ticket.State("in-review"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
		Handoff:     "**Current state:**\nready\n\n**Decisions made:**\nnone\n\n**Files touched:**\n- x\n\n**Remaining work:**\n- y\n\n**AC status:**\n- done",
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-186", "--state", "done"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected privileged rejection for done transition without secure surface")
	}

	rootCmd.SetArgs([]string{"secure", "unlock", "--password", "pw-1", "--ttl", "5m"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("secure unlock failed: %v", err)
	}

	rootCmd.SetArgs([]string{"update", "TKT-186", "--state", "done", "--ticket", "TKT-186", "--yes"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("secure done transition failed: %v", err)
	}

	got, err := s.GetTicket(context.Background(), "TKT-186")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if got.State != "done" {
		t.Fatalf("expected done state, got %s", got.State)
	}

	session := security.NewSessionManager(tmpHome)
	if err := session.RequireActive(tmpDir); err != nil {
		t.Fatalf("expected secure session to remain active, got: %v", err)
	}
}

func TestUpdateCmd_RejectsEmptyTitle(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-001",
		Title:       "Current title",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-001", "--title", "   "})
	if err := rootCmd.Execute(); err == nil || !strings.Contains(err.Error(), "title cannot be empty") {
		t.Fatalf("expected empty title error, got %v", err)
	}
}

func TestUpdateCmd_RejectsEmptyState(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-001",
		Title:       "Current title",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-001", "--state", " "})
	if err := rootCmd.Execute(); err == nil || !strings.Contains(err.Error(), "state cannot be empty") {
		t.Fatalf("expected empty state error, got %v", err)
	}
}
