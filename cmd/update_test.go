package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/claim"
	docketgit "github.com/leomorpho/docket/internal/git"
	"github.com/leomorpho/docket/internal/runstate"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func initGitRepoForUpdateTest(t *testing.T, repoRoot string) {
	t.Helper()
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "config", "user.name", "Docket Test")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config user.name failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "config", "user.email", "docket-test@example.com")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config user.email failed: %v\n%s", err, out)
	}
}

func updateRunnableDescription() string {
	return "Likely paths: cmd/update.go and cmd/update_test.go. Verify commands: go test ./cmd -run TestUpdateCmd -count=1. Out of scope: unrelated security cleanup or scheduler behavior. This fixture contains enough execution context to satisfy runnable-state validation during update command coverage."
}

func updateRunnableAC() []ticket.AcceptanceCriterion {
	return []ticket.AcceptanceCriterion{
		{Description: "A1"},
		{Description: "A2"},
	}
}

func updateCompletedAC() []ticket.AcceptanceCriterion {
	return []ticket.AcceptanceCriterion{
		{Description: "A1", Done: true, Evidence: "ok"},
		{Description: "A2", Done: true, Evidence: "ok"},
	}
}

func updateStructuredHandoff(currentState, remaining string) string {
	if strings.TrimSpace(remaining) == "" {
		remaining = "none"
	}
	return "**Current state:**\n" + currentState + "\n\n**Decisions made:**\nnone\n\n**Files touched:**\n- x\n\n**Remaining work:**\n- " + remaining + "\n\n**AC status:**\n- complete"
}

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
		State:       ticket.State("draft"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: updateRunnableDescription(),
		AC:          updateRunnableAC(),
	}
	s.CreateTicket(ctx, tick)

	// 1. Update state (draft -> ready)
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)

	updateState = "ready"
	rootCmd.SetArgs([]string{"update", "TKT-001", "--state", "ready"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("update state failed: %v", err)
	}
	if !strings.Contains(b.String(), "state draft → ready") {
		t.Errorf("expected state transition message, got: %s", b.String())
	}

	updated, _ := s.GetTicket(ctx, "TKT-001")
	if updated.State != ticket.State("ready") {
		t.Errorf("expected state ready, got %s", updated.State)
	}

	// 2. Invalid transition (ready -> validated)
	updateState = "validated"
	rootCmd.SetArgs([]string{"update", "TKT-001", "--state", "validated"})
	if err := rootCmd.Execute(); err == nil {
		t.Error("expected error for invalid transition ready -> validated, got nil")
	} else if !strings.Contains(strings.ToLower(err.Error()), "cannot transition") {
		t.Fatalf("expected invalid transition rejection for ready -> validated, got: %v", err)
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
	initGitRepoForUpdateTest(t, tmpDir)
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	cfg := ticket.DefaultConfig()
	cfg.SecurityEnforcement = true
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	tick := &ticket.Ticket{
		ID:          "TKT-001",
		Title:       "Needs Handoff",
		State:       ticket.State("ready"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: updateRunnableDescription(),
		AC:          updateRunnableAC(),
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

func TestUpdateCmd_RejectsNonLeafExecutionBlocker(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for _, tk := range []*ticket.Ticket{
		{
			ID:          "TKT-001",
			Seq:         1,
			Title:       "Parent blocker",
			State:       ticket.State("draft"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "Parent blocker",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		},
		{
			ID:          "TKT-002",
			Seq:         2,
			Title:       "Child under blocker",
			Parent:      "TKT-001",
			State:       ticket.State("draft"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "Child ticket",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		},
		{
			ID:          "TKT-003",
			Seq:         3,
			Title:       "Target",
			State:       ticket.State("draft"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "Target ticket",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		},
	} {
		if err := s.CreateTicket(ctx, tk); err != nil {
			t.Fatalf("create %s: %v", tk.ID, err)
		}
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-003", "--blocked-by", "TKT-001"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected update to reject non-leaf blocker")
	}
	if !strings.Contains(err.Error(), "must be a leaf ticket") {
		t.Fatalf("expected leaf-blocker error, got %v", err)
	}

	updated, getErr := s.GetTicket(ctx, "TKT-003")
	if getErr != nil {
		t.Fatalf("get target: %v", getErr)
	}
	if len(updated.BlockedBy) != 0 {
		t.Fatalf("expected target blockers unchanged, got %v", updated.BlockedBy)
	}
}

func TestUpdateCmd_AllowsInProgressBackToBacklog(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	cfg := ticket.DefaultConfig()
	cfg.SecurityEnforcement = true
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	tick := &ticket.Ticket{
		ID:          "TKT-001",
		Title:       "Deferred work",
		State:       ticket.State("running"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: updateRunnableDescription(),
		AC:          updateRunnableAC(),
	}
	if err := s.CreateTicket(ctx, tick); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"update", "TKT-001", "--state", "draft"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected running -> draft transition to succeed, got: %v", err)
	}
	if !strings.Contains(b.String(), "state running → draft") {
		t.Fatalf("expected transition output, got: %s", b.String())
	}

	updated, err := s.GetTicket(ctx, "TKT-001")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if updated.State != ticket.State("draft") {
		t.Fatalf("expected draft state, got %s", updated.State)
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
		State:       ticket.State("draft"),
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
		Next:   []string{"ready", "archived"},
	}
	ready := cfg.States["ready"]
	ready.Next = append(ready.Next, "stale")
	cfg.States["ready"] = ready
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
		State:       ticket.State("ready"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: updateRunnableDescription(),
		AC:          updateRunnableAC(),
	}); err != nil {
		t.Fatalf("CreateTicket parent failed: %v", err)
	}
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-002",
		Seq:         2,
		Title:       "Child",
		Parent:      "TKT-001",
		State:       ticket.State("ready"),
		Priority:    2,
		CreatedAt:   now.Add(time.Minute),
		UpdatedAt:   now.Add(time.Minute),
		CreatedBy:   "me",
		Description: updateRunnableDescription(),
		AC:          updateRunnableAC(),
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

func TestUpdateCmd_ValidatedTransitionSetsCompletedAt(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-186",
		Seq:         186,
		Title:       "Validated transition",
		State:       ticket.State("running"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: updateRunnableDescription(),
		AC:          updateCompletedAC(),
		Handoff:     updateStructuredHandoff("running", "none"),
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-186", "--state", "validated"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("validated transition failed: %v", err)
	}

	got, err := s.GetTicket(context.Background(), "TKT-186")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if got.State != "validated" {
		t.Fatalf("expected validated state, got %s", got.State)
	}
	if got.CompletedAt.IsZero() {
		t.Fatalf("expected validated transition to set completed_at")
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
		Description: updateRunnableDescription(),
		Handoff:     updateStructuredHandoff("queued", "none"),
		AC:          updateCompletedAC(),
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

func TestUpdateCmd_CustomWorkflowCompletedStateUsesConfiguredCompletedState(t *testing.T) {
	tmpDir := t.TempDir()
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
		Description: updateRunnableDescription(),
		Handoff:     updateStructuredHandoff("qa", "none"),
		AC:          updateCompletedAC(),
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-441", "--state", "shipped"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected qa -> shipped transition to succeed, got: %v", err)
	}

	got, err := s.GetTicket(context.Background(), "TKT-441")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if got.State != "shipped" {
		t.Fatalf("expected shipped state, got %s", got.State)
	}
	if got.CompletedAt.IsZero() {
		t.Fatalf("expected shipped transition to set completed_at")
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
	cfg.SecurityEnforcement = true
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-198",
		Seq:         198,
		Title:       "Managed run linkage",
		State:       ticket.State("running"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
		Description: updateRunnableDescription(),
		AC:          updateCompletedAC(),
		Handoff:     updateStructuredHandoff("running", "none"),
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	ns := runstate.New(defaultRuntimeNamespaceRoot(tmpDir))
	worktreePath := filepath.Join(tmpDir, "wt", "TKT-198")
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatalf("mkdir worktree path failed: %v", err)
	}
	if err := ns.RecordRunStart(tmpDir, "TKT-198", "agent:test", worktreePath, "HEAD", "hash-198"); err != nil {
		t.Fatalf("RecordRunStart failed: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-198", "--state", "validated"})
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
	rootCmd.SetArgs([]string{"update", "TKT-198", "--state", "validated"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected validated transition after linked commit, got: %v", err)
	}
}

func TestUpdateCmd_ManagedRunLinkageWarningOnlyByDefault(t *testing.T) {
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
	cfg.SecurityEnforcement = false
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-198B",
		Seq:         198,
		Title:       "Managed run linkage warning-only",
		State:       ticket.State("running"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
		Description: updateRunnableDescription(),
		AC:          updateCompletedAC(),
		Handoff:     updateStructuredHandoff("running", "none"),
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	ns := runstate.New(defaultRuntimeNamespaceRoot(tmpDir))
	worktreePath := filepath.Join(tmpDir, "wt", "TKT-198B")
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatalf("mkdir worktree path failed: %v", err)
	}
	if err := ns.RecordRunStart(tmpDir, "TKT-198B", "agent:test", worktreePath, "HEAD", "hash-198B"); err != nil {
		t.Fatalf("RecordRunStart failed: %v", err)
	}

	errBuf := new(bytes.Buffer)
	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs([]string{"update", "TKT-198B", "--state", "validated"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected warning-only linkage mode to proceed, got: %v", err)
	}
	if !strings.Contains(errBuf.String(), "warning: managed run") {
		t.Fatalf("expected managed-run linkage warning, got: %s", errBuf.String())
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

	cfg := ticket.DefaultConfig()
	cfg.SecurityEnforcement = true
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-287",
		Seq:         287,
		Title:       "Managed branch drift",
		State:       ticket.State("running"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
		Description: updateRunnableDescription(),
		AC:          updateCompletedAC(),
		Handoff:     updateStructuredHandoff("running", "none"),
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	worktreePath := filepath.Join(tmpDir, "wt", "TKT-287")
	if err := docketgit.CreateWorktree(tmpDir, "TKT-287", "docket/TKT-287", worktreePath); err != nil {
		t.Fatalf("CreateWorktree failed: %v", err)
	}

	ns := runstate.New(defaultRuntimeNamespaceRoot(tmpDir))
	if err := ns.RecordRunStart(tmpDir, "TKT-287", "agent:test", worktreePath, "docket/TKT-287", "hash-287"); err != nil {
		t.Fatalf("RecordRunStart failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "work.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	runGitSession(t, tmpDir, "add", ".")
	runGitSession(t, tmpDir, "commit", "-m", "feat: drifted commit\n\nTicket: TKT-287")

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-287", "--state", "validated"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected validated transition after auto-repair, got: %v", err)
	}
	ok, err := docketgit.HasTicketTrailerSince(tmpDir, "docket/TKT-287", "TKT-287", now.Add(-time.Minute).Format(time.RFC3339))
	if err != nil {
		t.Fatalf("check repaired branch failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected repaired branch docket/TKT-287 to include ticket trailer")
	}
}

func TestUpdateCmd_ManagedRunValidatedPassesFromBoundWorktree(t *testing.T) {
	tmpDir := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	t.Setenv("DOCKET_AGENT_ID", "test-agent")
	docketHome = ""
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
		ID:          "TKT-270",
		Seq:         270,
		Title:       "Bound worktree review transition",
		State:       ticket.State("running"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
		Description: updateRunnableDescription(),
		AC:          updateCompletedAC(),
		Handoff:     updateStructuredHandoff("running", "none"),
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}
	runGitSession(t, tmpDir, "add", ".")
	runGitSession(t, tmpDir, "commit", "-m", "chore: setup ticket")

	worktreePath := filepath.Join(tmpDir, "wt", "TKT-270")
	if err := docketgit.CreateWorktree(tmpDir, "TKT-270", "docket/TKT-270", worktreePath); err != nil {
		t.Fatalf("CreateWorktree failed: %v", err)
	}

	ns := runstate.New(defaultRuntimeNamespaceRoot(tmpDir))
	if err := ns.RecordRunStart(tmpDir, "TKT-270", "agent:test-agent", worktreePath, "docket/TKT-270", "hash-270"); err != nil {
		t.Fatalf("RecordRunStart failed: %v", err)
	}
	if err := claim.Claim(tmpDir, "TKT-270", worktreePath, "agent:test-agent"); err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(worktreePath, "work.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write file in worktree failed: %v", err)
	}
	runGitSession(t, worktreePath, "add", ".")
	runGitSession(t, worktreePath, "commit", "-m", "feat: worktree review transition\n\nTicket: TKT-270")

	oldRepo := repo
	repo = worktreePath
	t.Cleanup(func() { repo = oldRepo })

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-270", "--state", "validated"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected validated transition from bound worktree, got: %v", err)
	}

	mainStore := local.New(tmpDir)
	got, err := mainStore.GetTicket(context.Background(), "TKT-270")
	if err != nil {
		t.Fatalf("load ticket from main checkout failed: %v", err)
	}
	if got == nil || got.State != "validated" {
		t.Fatalf("expected main checkout ticket state validated after merge-back, got %#v", got)
	}
}

func TestUpdateCmd_ManagedRunValidatedRecoversWhenManifestBranchMissing(t *testing.T) {
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

	cfg := ticket.DefaultConfig()
	cfg.SecurityEnforcement = true
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-496",
		Seq:         496,
		Title:       "Missing managed branch fallback",
		State:       ticket.State("running"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
		Description: updateRunnableDescription(),
		AC:          updateCompletedAC(),
		Handoff:     updateStructuredHandoff("running", "none"),
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}
	runGitSession(t, tmpDir, "add", ".")
	runGitSession(t, tmpDir, "commit", "-m", "chore: setup ticket")

	worktreePath := filepath.Join(tmpDir, "wt", "TKT-496")
	if err := docketgit.CreateWorktree(tmpDir, "TKT-496", "scratch/TKT-496", worktreePath); err != nil {
		t.Fatalf("CreateWorktree failed: %v", err)
	}

	ns := runstate.New(defaultRuntimeNamespaceRoot(tmpDir))
	if err := ns.RecordRunStart(tmpDir, "TKT-496", "agent:test-agent", worktreePath, "docket/TKT-496", "hash-496"); err != nil {
		t.Fatalf("RecordRunStart failed: %v", err)
	}
	if err := claim.Claim(tmpDir, "TKT-496", worktreePath, "agent:test-agent"); err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(worktreePath, "feature.txt"), []byte("from worktree\n"), 0o644); err != nil {
		t.Fatalf("write worktree file failed: %v", err)
	}
	runGitSession(t, worktreePath, "add", ".")
	runGitSession(t, worktreePath, "commit", "-m", "feat: worktree fallback\n\nTicket: TKT-496")

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-496", "--state", "validated"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected validated transition to recover when manifest branch is missing, got: %v", err)
	}

	mainStore := local.New(tmpDir)
	got, err := mainStore.GetTicket(context.Background(), "TKT-496")
	if err != nil {
		t.Fatalf("load ticket from main checkout failed: %v", err)
	}
	if got == nil || got.State != "validated" {
		t.Fatalf("expected main checkout ticket state validated after fallback merge-back, got %#v", got)
	}

	featureData, err := os.ReadFile(filepath.Join(tmpDir, "feature.txt"))
	if err != nil {
		t.Fatalf("read merged feature file failed: %v", err)
	}
	if strings.TrimSpace(string(featureData)) != "from worktree" {
		t.Fatalf("expected fallback merge to include worktree commit, got %q", string(featureData))
	}
}

func TestUpdateCmd_ManagedRunValidatedAutostashesDirtyMainCheckout(t *testing.T) {
	tmpDir := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	t.Setenv("DOCKET_AGENT_ID", "test-agent")
	docketHome = ""
	format = "human"

	runGitSession(t, tmpDir, "init")
	runGitSession(t, tmpDir, "config", "user.email", "test@example.com")
	runGitSession(t, tmpDir, "config", "user.name", "Test User")
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "docs", "guide.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write guide failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed file failed: %v", err)
	}
	runGitSession(t, tmpDir, "add", ".")
	runGitSession(t, tmpDir, "commit", "-m", "chore: seed")

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-271",
		Seq:         271,
		Title:       "Bound worktree review with dirty main checkout",
		State:       ticket.State("running"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
		Description: updateRunnableDescription(),
		AC:          updateCompletedAC(),
		Handoff:     updateStructuredHandoff("running", "none"),
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}
	runGitSession(t, tmpDir, "add", ".")
	runGitSession(t, tmpDir, "commit", "-m", "chore: setup ticket")

	worktreePath := filepath.Join(tmpDir, "wt", "TKT-271")
	if err := docketgit.CreateWorktree(tmpDir, "TKT-271", "docket/TKT-271", worktreePath); err != nil {
		t.Fatalf("CreateWorktree failed: %v", err)
	}

	ns := runstate.New(defaultRuntimeNamespaceRoot(tmpDir))
	if err := ns.RecordRunStart(tmpDir, "TKT-271", "agent:test-agent", worktreePath, "docket/TKT-271", "hash-271"); err != nil {
		t.Fatalf("RecordRunStart failed: %v", err)
	}
	if err := claim.Claim(tmpDir, "TKT-271", worktreePath, "agent:test-agent"); err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(worktreePath, "feature.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write feature file in worktree failed: %v", err)
	}
	runGitSession(t, worktreePath, "add", ".")
	runGitSession(t, worktreePath, "commit", "-m", "feat: worktree review transition with dirty main\n\nTicket: TKT-271")

	dirtyGuidePath := filepath.Join(tmpDir, "docs", "guide.md")
	if err := os.WriteFile(dirtyGuidePath, []byte("base\nlocal dirty change\n"), 0o644); err != nil {
		t.Fatalf("dirty guide failed: %v", err)
	}

	oldRepo := repo
	repo = worktreePath
	t.Cleanup(func() { repo = oldRepo })

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-271", "--state", "validated"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected validated transition from bound worktree with dirty main checkout, got: %v", err)
	}

	mainStore := local.New(tmpDir)
	got, err := mainStore.GetTicket(context.Background(), "TKT-271")
	if err != nil {
		t.Fatalf("load ticket from main checkout failed: %v", err)
	}
	if got == nil || got.State != "validated" {
		t.Fatalf("expected main checkout ticket state validated after merge-back, got %#v", got)
	}
	featureData, err := os.ReadFile(filepath.Join(tmpDir, "feature.txt"))
	if err != nil {
		t.Fatalf("read merged feature file failed: %v", err)
	}
	if strings.TrimSpace(string(featureData)) != "x" {
		t.Fatalf("expected merged feature file in main checkout, got %q", string(featureData))
	}
	dirtyData, err := os.ReadFile(dirtyGuidePath)
	if err != nil {
		t.Fatalf("read dirty guide failed: %v", err)
	}
	if !strings.Contains(string(dirtyData), "local dirty change") {
		t.Fatalf("expected dirty main-checkout edits to survive merge-back, got %q", string(dirtyData))
	}
}

func TestUpdateCmd_ManagedRunWarnsOnStaleRunManifestByDefault(t *testing.T) {
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
	cfg.SecurityEnforcement = false
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-209",
		Seq:         209,
		Title:       "Managed run stale manifest",
		State:       ticket.State("running"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
		Description: updateRunnableDescription(),
		AC:          updateCompletedAC(),
		Handoff:     updateStructuredHandoff("running", "none"),
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	ns := runstate.New(defaultRuntimeNamespaceRoot(tmpDir))
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

	errBuf := new(bytes.Buffer)
	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs([]string{"update", "TKT-209", "--state", "validated"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected stale run manifest warning-only behavior, got: %v", err)
	}
	if !strings.Contains(errBuf.String(), "warning: run manifest validation failed") {
		t.Fatalf("expected stale run manifest warning, got: %s", errBuf.String())
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
		State:       ticket.State("draft"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: "Simple draft description",
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
		State:       ticket.State("draft"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: "Simple draft description",
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

func TestUpdateDoesNotAutoTransitionReadyTickets(t *testing.T) {
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
	handoff := updateStructuredHandoff("running", "none")
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-901",
		Seq:         901,
		Title:       "Auto transition ready",
		State:       ticket.State("running"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: updateRunnableDescription(),
		Handoff:     handoff,
		AC:          updateCompletedAC(),
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"update", "TKT-901", "--title", "Updated auto transition title"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	got, err := s.GetTicket(context.Background(), "TKT-901")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if got.State != ticket.State("running") {
		t.Fatalf("expected ticket to remain running, got %s", got.State)
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
		State:       ticket.State("running"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: updateRunnableDescription(),
		AC: []ticket.AcceptanceCriterion{
			{Description: "A1", Done: false},
			{Description: "A2", Done: true, Evidence: "ok"},
		},
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"update", "TKT-902", "--title", "Updated not-ready title"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	got, err := s.GetTicket(context.Background(), "TKT-902")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if got.State != ticket.State("running") {
		t.Fatalf("expected ticket to remain running, got %s", got.State)
	}
}

func TestUpdateDoesNotAutoTransitionReadyTicketsWithCustomWorkflow(t *testing.T) {
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
	handoff := updateStructuredHandoff("testing", "none")
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-903",
		Seq:         903,
		Title:       "Custom auto transition ready",
		State:       "testing",
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: updateRunnableDescription(),
		Handoff:     handoff,
		AC:          updateCompletedAC(),
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"update", "TKT-903", "--title", "Updated custom auto transition"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	got, err := s.GetTicket(context.Background(), "TKT-903")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if got.State != ticket.State("testing") {
		t.Fatalf("expected ticket to remain testing, got %s", got.State)
	}
}
