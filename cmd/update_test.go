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
	} else if !strings.Contains(err.Error(), "--ticket is required") {
		t.Fatalf("expected human done transition to require privileged surface, got: %v", err)
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

func TestUpdateCmd_AllowsInProgressBackToBacklog(t *testing.T) {
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
		Title:       "Deferred work",
		State:       ticket.State("in-progress"),
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

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"update", "TKT-001", "--state", "backlog"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected in-progress -> backlog transition to succeed, got: %v", err)
	}
	if !strings.Contains(b.String(), "state in-progress → backlog") {
		t.Fatalf("expected transition output, got: %s", b.String())
	}

	updated, err := s.GetTicket(ctx, "TKT-001")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if updated.State != ticket.State("backlog") {
		t.Fatalf("expected backlog state, got %s", updated.State)
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
	} else if !strings.Contains(err.Error(), "--ticket is required") {
		t.Fatalf("expected secure-surface guidance for human done transition, got: %v", err)
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
	if got.CompletedAt.IsZero() {
		t.Fatalf("expected done transition to set completed_at")
	}

	session := security.NewSessionManager(tmpHome)
	if err := session.RequireActive(tmpDir); err != nil {
		t.Fatalf("expected secure session to remain active, got: %v", err)
	}
}

func TestUpdateCmd_AgentDoneTransitionRedirectsToInReview(t *testing.T) {
	tmpDir := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	t.Setenv("DOCKET_AGENT_ID", "codex-test")
	docketHome = ""
	repo = tmpDir
	format = "human"

	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-333",
		Seq:         333,
		Title:       "Agent closure attempt",
		State:       ticket.State("in-review"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A", Done: true, Evidence: "ok"}},
		Handoff:     "**Current state:**\nready\n\n**Decisions made:**\nnone\n\n**Files touched:**\n- x\n\n**Remaining work:**\n- none\n\n**AC status:**\n- done",
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-333", "--state", "done", "--ticket", "TKT-333", "--yes"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected agent done transition to be redirected")
	} else if !strings.Contains(err.Error(), "human-only") || !strings.Contains(err.Error(), "in-review") {
		t.Fatalf("expected agent done transition guidance to point at in-review, got: %v", err)
	}
}

func TestUpdateCmd_CustomWorkflowUsesConfiguredActiveAndReviewStates(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	runGitSession(t, tmpDir, "init")
	runGitSession(t, tmpDir, "config", "user.email", "test@example.com")
	runGitSession(t, tmpDir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(tmpDir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed file failed: %v", err)
	}
	runGitSession(t, tmpDir, "add", ".")
	runGitSession(t, tmpDir, "commit", "-m", "chore: seed")

	cfg := &ticket.Config{
		Backend: "local",
		States: map[string]ticket.StateConfig{
			"queued": {
				Label:            "Queued",
				Open:             true,
				Column:           0,
				Next:             []string{"building"},
				Roles:            []string{"intake"},
				Startable:        true,
				BlocksDependents: true,
			},
			"building": {
				Label:            "Building",
				Open:             true,
				Column:           1,
				Next:             []string{"qa"},
				Roles:            []string{"active"},
				BlocksDependents: true,
			},
			"qa": {
				Label:            "QA",
				Open:             true,
				Column:           2,
				Next:             []string{"shipped"},
				Roles:            []string{"review"},
				Reviewable:       true,
				BlocksDependents: true,
			},
			"shipped": {
				Label:    "Shipped",
				Open:     false,
				Column:   3,
				Next:     []string{},
				Roles:    []string{"completed"},
				Terminal: true,
			},
		},
		DefaultState: "queued",
	}
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-440",
		Seq:         440,
		Title:       "Custom workflow lifecycle",
		State:       "queued",
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "D",
		Handoff:     "**Current state:**\nqueued\n\n**Decisions made:**\nnone\n\n**Files touched:**\n- x\n\n**Remaining work:**\n- y\n\n**AC status:**\n- complete",
		AC: []ticket.AcceptanceCriterion{
			{Description: "A1", Done: true, Evidence: "ok"},
			{Description: "A2", Done: true, Evidence: "ok"},
		},
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-440", "--state", "building"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected queued -> building transition to succeed, got: %v", err)
	}
	active, err := s.GetTicket(context.Background(), "TKT-440")
	if err != nil {
		t.Fatalf("GetTicket after active transition failed: %v", err)
	}
	if active.State != "building" {
		t.Fatalf("expected active state building, got %s", active.State)
	}
	if active.StartedAt.IsZero() {
		t.Fatalf("expected StartedAt to be set when entering configured active state")
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-440", "--state", "qa"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected building -> qa transition to succeed, got: %v", err)
	}
	review, err := s.GetTicket(context.Background(), "TKT-440")
	if err != nil {
		t.Fatalf("GetTicket after review transition failed: %v", err)
	}
	if review.State != "qa" {
		t.Fatalf("expected configured review state qa, got %s", review.State)
	}
}

func TestUpdateCmd_CustomWorkflowCompletedStateRequiresPrivilegedSurface(t *testing.T) {
	tmpDir := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	docketHome = ""
	repo = tmpDir
	format = "human"

	cfg := &ticket.Config{
		Backend: "local",
		States: map[string]ticket.StateConfig{
			"queued": {
				Label:            "Queued",
				Open:             true,
				Column:           0,
				Next:             []string{"building"},
				Roles:            []string{"intake"},
				Startable:        true,
				BlocksDependents: true,
			},
			"building": {
				Label:            "Building",
				Open:             true,
				Column:           1,
				Next:             []string{"qa"},
				Roles:            []string{"active"},
				BlocksDependents: true,
			},
			"qa": {
				Label:            "QA",
				Open:             true,
				Column:           2,
				Next:             []string{"shipped"},
				Roles:            []string{"review"},
				Reviewable:       true,
				BlocksDependents: true,
			},
			"shipped": {
				Label:    "Shipped",
				Open:     false,
				Column:   3,
				Next:     []string{},
				Roles:    []string{"completed"},
				Terminal: true,
			},
		},
		DefaultState: "queued",
	}
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-441",
		Seq:         441,
		Title:       "Custom completed state",
		State:       "qa",
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "D",
		Handoff:     "**Current state:**\nqa\n\n**Decisions made:**\nnone\n\n**Files touched:**\n- x\n\n**Remaining work:**\n- none\n\n**AC status:**\n- done",
		AC: []ticket.AcceptanceCriterion{
			{Description: "A1", Done: true, Evidence: "ok"},
			{Description: "A2", Done: true, Evidence: "ok"},
		},
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-441", "--state", "shipped"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected privileged rejection for custom completed state")
	} else if !strings.Contains(err.Error(), "--ticket is required") {
		t.Fatalf("expected secure-surface guidance for custom completed state, got: %v", err)
	}
}

func TestUpdateCmd_ManagedRunRequiresCommitLinkage(t *testing.T) {
	tmpDir := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	docketHome = ""
	repo = tmpDir
	format = "human"

	runGitSession(t, tmpDir, "init")
	runGitSession(t, tmpDir, "config", "user.email", "test@example.com")
	runGitSession(t, tmpDir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(tmpDir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed file failed: %v", err)
	}
	runGitSession(t, tmpDir, "add", ".")
	runGitSession(t, tmpDir, "commit", "-m", "chore: seed")

	cfg := ticket.DefaultConfig()
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-198",
		Seq:         198,
		Title:       "Managed run linkage",
		State:       ticket.State("in-progress"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
		Handoff:     "handoff",
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	ns := security.NewRepoNamespaceStore(tmpHome)
	worktreePath := filepath.Join(tmpDir, "wt", "TKT-198")
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatalf("mkdir worktree path failed: %v", err)
	}
	if err := ns.RecordRunStart(tmpDir, "TKT-198", "agent:test", worktreePath, "HEAD", "hash-198"); err != nil {
		t.Fatalf("RecordRunStart failed: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-198", "--state", "in-review"})
	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "no commit on HEAD references Ticket: TKT-198") {
		t.Fatalf("expected commit-linkage rejection, got: %v", err)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "work.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	runGitSession(t, tmpDir, "add", ".")
	runGitSession(t, tmpDir, "commit", "-m", "feat: managed run linkage\n\nTicket: TKT-198")

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-198", "--state", "in-review"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected in-review transition after linked commit, got: %v", err)
	}
}

func TestUpdateCmd_ManagedRunAutoRepairsBoundBranchDrift(t *testing.T) {
	tmpDir := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	t.Setenv("DOCKET_AGENT_ID", "test-agent")
	docketHome = ""
	repo = tmpDir
	format = "human"

	runGitSession(t, tmpDir, "init")
	runGitSession(t, tmpDir, "config", "user.email", "test@example.com")
	runGitSession(t, tmpDir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(tmpDir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed file failed: %v", err)
	}
	runGitSession(t, tmpDir, "add", ".")
	runGitSession(t, tmpDir, "commit", "-m", "chore: seed")

	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-287",
		Seq:         287,
		Title:       "Managed branch drift",
		State:       ticket.State("in-progress"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
		Handoff:     "handoff",
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	worktreePath := filepath.Join(tmpDir, "wt", "TKT-287")
	if err := docketgit.CreateWorktree(tmpDir, "TKT-287", "docket/TKT-287", worktreePath); err != nil {
		t.Fatalf("CreateWorktree failed: %v", err)
	}

	ns := security.NewRepoNamespaceStore(tmpHome)
	if err := ns.RecordRunStart(tmpDir, "TKT-287", "agent:test", worktreePath, "docket/TKT-287", "hash-287"); err != nil {
		t.Fatalf("RecordRunStart failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "work.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	runGitSession(t, tmpDir, "add", ".")
	runGitSession(t, tmpDir, "commit", "-m", "feat: drifted commit\n\nTicket: TKT-287")

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-287", "--state", "in-review"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected in-review transition after auto-repair, got: %v", err)
	}
	ok, err := docketgit.HasTicketTrailerSince(tmpDir, "docket/TKT-287", "TKT-287", now.Add(-time.Minute).Format(time.RFC3339))
	if err != nil {
		t.Fatalf("check repaired branch failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected repaired branch docket/TKT-287 to include ticket trailer")
	}
}

func TestUpdateCmd_ManagedRunRejectsStaleRunManifest(t *testing.T) {
	tmpDir := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	docketHome = ""
	repo = tmpDir
	format = "human"

	runGitSession(t, tmpDir, "init")
	runGitSession(t, tmpDir, "config", "user.email", "test@example.com")
	runGitSession(t, tmpDir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(tmpDir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed file failed: %v", err)
	}
	runGitSession(t, tmpDir, "add", ".")
	runGitSession(t, tmpDir, "commit", "-m", "chore: seed")

	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-209",
		Seq:         209,
		Title:       "Managed run stale manifest",
		State:       ticket.State("in-progress"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
		Handoff:     "handoff",
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	ns := security.NewRepoNamespaceStore(tmpHome)
	worktreePath := filepath.Join(tmpDir, "wt", "TKT-209")
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatalf("mkdir worktree path failed: %v", err)
	}
	if err := ns.RecordRunStart(tmpDir, "TKT-209", "agent:test", worktreePath, "HEAD", "hash-209"); err != nil {
		t.Fatalf("RecordRunStart failed: %v", err)
	}
	run, ok, err := ns.GetRunManifest(tmpDir, "TKT-209")
	if err != nil || !ok {
		t.Fatalf("GetRunManifest failed: ok=%v err=%v", ok, err)
	}
	run.StartedAt = time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339Nano)
	repoID, nsDir, err := ns.EnsureRepoNamespace(tmpDir)
	if err != nil {
		t.Fatalf("EnsureRepoNamespace failed: %v", err)
	}
	run.RepoID = repoID
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		t.Fatalf("marshal stale run manifest failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nsDir, "runs", "TKT-209.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write stale run manifest failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "work.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	runGitSession(t, tmpDir, "add", ".")
	runGitSession(t, tmpDir, "commit", "-m", "feat: stale manifest\n\nTicket: TKT-209")

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-209", "--state", "in-review"})
	err = rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "run manifest validation failed") {
		t.Fatalf("expected stale run manifest rejection, got: %v", err)
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

func TestEnforceStructuredACClosureGateRejectsIncompleteHumanVerification(t *testing.T) {
	tkt := &ticket.Ticket{
		ID: "TKT-900",
		AC: []ticket.AcceptanceCriterion{
			{
				Description: "Manual CLI verification",
				Kind:        "human",
				AppliesTo:   []string{"cli"},
				VerificationSteps: []string{
					"Run command",
				},
				Done: false,
			},
		},
	}
	err := enforceStructuredACClosureGate(tkt)
	if err == nil || !strings.Contains(err.Error(), "must be marked done") {
		t.Fatalf("expected closure gate rejection for incomplete human verification, got %v", err)
	}
}

func TestUpdateAutoTransitionsReviewReadyTickets(t *testing.T) {
	tmpDir := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	docketHome = ""
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	handoff := "**Current state:** in-progress\n**Decisions made:** none\n**Files touched:** x\n**Remaining work:** review\n**AC status:** complete"
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-901",
		Seq:         901,
		Title:       "Auto transition ready",
		State:       ticket.State("in-progress"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "D",
		Handoff:     handoff,
		AC: []ticket.AcceptanceCriterion{
			{Description: "A1", Done: true, Evidence: "done"},
			{Description: "A2", Done: true, Evidence: "done"},
		},
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"update", "TKT-901", "--desc", "updated desc"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	got, err := s.GetTicket(context.Background(), "TKT-901")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if got.State != ticket.State("in-review") {
		t.Fatalf("expected auto transition to in-review, got %s", got.State)
	}
	if !strings.Contains(out.String(), "Auto-transitioned TKT-901") {
		t.Fatalf("expected auto-transition diagnostic, got: %s", out.String())
	}
}

func TestUpdateDoesNotAutoTransitionWhenReadinessFails(t *testing.T) {
	tmpDir := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	docketHome = ""
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-902",
		Seq:         902,
		Title:       "Auto transition not ready",
		State:       ticket.State("in-progress"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "D",
		AC: []ticket.AcceptanceCriterion{
			{Description: "A1", Done: false},
		},
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"update", "TKT-902", "--desc", "updated desc"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	got, err := s.GetTicket(context.Background(), "TKT-902")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if got.State != ticket.State("in-progress") {
		t.Fatalf("expected ticket to remain in-progress, got %s", got.State)
	}
	if !strings.Contains(out.String(), "Auto-review skipped for TKT-902") {
		t.Fatalf("expected skip diagnostic, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "acceptance criteria incomplete") || !strings.Contains(out.String(), "handoff missing") {
		t.Fatalf("expected readiness-failure reasons in diagnostic, got: %s", out.String())
	}
}

func TestUpdateAutoTransitionsReviewReadyTicketsWithCustomWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	docketHome = ""
	repo = tmpDir
	format = "human"

	cfg := &ticket.Config{
		Backend: "local",
		States: map[string]ticket.StateConfig{
			"queued": {
				Label:            "Queued",
				Open:             true,
				Column:           0,
				Next:             []string{"coding"},
				Roles:            []string{"intake"},
				Startable:        true,
				BlocksDependents: true,
			},
			"coding": {
				Label:            "Coding",
				Open:             true,
				Column:           1,
				Next:             []string{"testing"},
				Roles:            []string{"active"},
				BlocksDependents: true,
			},
			"testing": {
				Label:            "Testing",
				Open:             true,
				Column:           2,
				Next:             []string{"qa"},
				Roles:            []string{"active"},
				BlocksDependents: true,
			},
			"qa": {
				Label:            "QA",
				Open:             true,
				Column:           3,
				Next:             []string{"shipped"},
				Roles:            []string{"review"},
				Reviewable:       true,
				BlocksDependents: true,
			},
			"shipped": {
				Label:    "Shipped",
				Open:     false,
				Column:   4,
				Next:     []string{},
				Roles:    []string{"completed"},
				Terminal: true,
			},
		},
		DefaultState:    "queued",
		DefaultPriority: 10,
		HandoffSections: []string{"Current state", "Decisions made"},
	}
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	handoff := "Current state\n\nDecisions made"
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-903",
		Seq:         903,
		Title:       "Custom auto transition ready",
		State:       "testing",
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "D",
		Handoff:     handoff,
		AC: []ticket.AcceptanceCriterion{
			{Description: "A1", Done: true, Evidence: "done"},
			{Description: "A2", Done: true, Evidence: "done"},
		},
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"update", "TKT-903", "--desc", "updated desc"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	got, err := s.GetTicket(context.Background(), "TKT-903")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if got.State != ticket.State("qa") {
		t.Fatalf("expected auto transition to qa, got %s", got.State)
	}
	if !strings.Contains(out.String(), "Auto-transitioned TKT-903") {
		t.Fatalf("expected auto-transition diagnostic, got: %s", out.String())
	}
}
