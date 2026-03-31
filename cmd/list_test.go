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

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestListCmd(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	listFull = false

	// 0. Setup store and tickets
	s := local.New(tmpDir)
	ticket.SaveConfig(tmpDir, ticket.DefaultConfig())
	ctx := context.Background()
	now := time.Now().UTC()

	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-001", Title: "Workable Ticket", State: ticket.State("todo"), Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{}}})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-004", Title: "Epic Ticket", State: ticket.State("todo"), Priority: 2, Labels: []string{"epic"}, CreatedAt: now.Add(2 * time.Hour), UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{}}})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-005", Title: "Program: Wrapper", State: ticket.State("todo"), Priority: 1, Labels: []string{"program", "topo:coordination"}, CreatedAt: now.Add(3 * time.Hour), UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{}}})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-002", Title: "Done Ticket", State: ticket.State("done"), Priority: 1, CreatedAt: now.Add(time.Hour), UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{}}})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-003", Title: "Archived Ticket", State: ticket.State("archived"), Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{}}})

	// 1. Default list shows workable tickets only.
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"list"})
	listState = "open" // Reset flag
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(b.String(), "TKT-001") {
		t.Errorf("expected TKT-001 in default list, got:\n%s", b.String())
	}
	if strings.Contains(b.String(), "TKT-004") {
		t.Errorf("expected epic ticket to be hidden in default list, got:\n%s", b.String())
	}
	if strings.Contains(b.String(), "TKT-005") {
		t.Errorf("expected coordination ticket to be hidden in default list, got:\n%s", b.String())
	}
	if strings.Contains(b.String(), "TKT-002") || strings.Contains(b.String(), "TKT-003") {
		t.Errorf("expected only workable tickets, but got:\n%s", b.String())
	}

	// 2. Explicit state filters bypass workable compaction.
	b.Reset()
	rootCmd.SetArgs([]string{"list", "--state", "done"})
	listState = "done"
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list done failed: %v", err)
	}
	if !strings.Contains(b.String(), "TKT-002") {
		t.Errorf("expected TKT-002 in done list, got:\n%s", b.String())
	}

	// 3. Format context
	b.Reset()
	rootCmd.SetArgs([]string{"list", "--format", "context"})
	format = "context"
	listState = "open"
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list context failed: %v", err)
	}
	if !strings.Contains(b.String(), "[TKT-001] P1 todo") {
		t.Errorf("expected compact context line, got:\n%s", b.String())
	}

	// 4. Format JSON
	b.Reset()
	rootCmd.SetArgs([]string{"list", "--format", "json"})
	format = "json"
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list json failed: %v", err)
	}
	var res []map[string]interface{}
	if err := json.Unmarshal(b.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if len(res) != 1 {
		t.Errorf("expected 1 workable ticket in JSON, got: %d", len(res))
	}
	if len(res) == 1 && res[0]["id"] != "TKT-001" {
		t.Errorf("expected TKT-001 in JSON, got %#v", res[0]["id"])
	}
}

func TestListCmd_GlobalSkillHintShownForHumanButNotJSON(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	listFull = false

	s := local.New(tmpDir)
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config failed: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID: "TKT-001", Seq: 1, Title: "Hint fixture", State: ticket.State("todo"), Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{Description: "x"}},
	}); err != nil {
		t.Fatalf("create ticket failed: %v", err)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list human failed: %v", err)
	}
	if !strings.Contains(out.String(), "Skill hint: use `docket skill invoke <skill-id>`") {
		t.Fatalf("expected global skill hint in human list output, got:\n%s", out.String())
	}

	out.Reset()
	rootCmd.SetArgs([]string{"list", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list json failed: %v", err)
	}
	if strings.Contains(out.String(), "Skill hint:") {
		t.Fatalf("expected no skill hint in list json output, got:\n%s", out.String())
	}
}

func TestListCmd_EmptyWorkableViewShowsStartableStates(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	listFull = false
	listState = "open"

	cfg := ticket.DefaultConfig()
	for name, state := range cfg.States {
		state.Startable = false
		cfg.States[name] = state
	}
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if !strings.Contains(out.String(), "No workable tickets found.") {
		t.Fatalf("expected empty workable-view message, got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Startable states in current config: none configured.") {
		t.Fatalf("expected startable-state summary in message, got:\n%s", out.String())
	}
}

func TestListCmd_EmptyWorkableViewExplainsBlockedBacklog(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	listFull = false
	listState = "open"

	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config failed: %v", err)
	}
	s := local.New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, tk := range []*ticket.Ticket{
		{
			ID:          "TKT-001",
			Seq:         1,
			Title:       "Blocker",
			State:       ticket.State("in-progress"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "me",
			Description: "D",
			AC:          []ticket.AcceptanceCriterion{{Description: "x"}},
		},
		{
			ID:          "TKT-002",
			Seq:         2,
			Title:       "Blocked backlog",
			State:       ticket.State("todo"),
			Priority:    2,
			BlockedBy:   []string{"TKT-001"},
			CreatedAt:   now.Add(time.Minute),
			UpdatedAt:   now.Add(time.Minute),
			CreatedBy:   "me",
			Description: "D",
			AC:          []ticket.AcceptanceCriterion{{Description: "x"}},
		},
	} {
		if err := s.CreateTicket(ctx, tk); err != nil {
			t.Fatalf("create ticket failed: %v", err)
		}
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if !strings.Contains(out.String(), "Queue warning: none are runnable right now") {
		t.Fatalf("expected blocked backlog explanation, got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Top unresolved blockers: TKT-001 x1") {
		t.Fatalf("expected top blocker summary, got:\n%s", out.String())
	}
}

func TestListCmd_ContextHonorsConfigForReviewBlockers(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "context"
	listFull = false

	cfg := ticket.DefaultConfig()
	review := cfg.States["in-review"]
	review.BlocksDependents = false
	cfg.States["in-review"] = review
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	s := local.New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID: "TKT-001", Seq: 1, Title: "Reviewed blocker", State: ticket.State("in-review"), Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{Description: "x"}},
	}); err != nil {
		t.Fatalf("create blocker failed: %v", err)
	}
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID: "TKT-002", Seq: 2, Title: "Dependent", State: ticket.State("todo"), Priority: 2, BlockedBy: []string{"TKT-001"},
		CreatedAt: now.Add(time.Minute), UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{Description: "x"}},
	}); err != nil {
		t.Fatalf("create dependent failed: %v", err)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"list", "--format", "context"})
	listState = "open"
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list context failed: %v", err)
	}
	if strings.Contains(out.String(), "BLOCKED by TKT-001") {
		t.Fatalf("expected in-review blocker to be treated as resolved by config, got:\n%s", out.String())
	}
}

func TestListCmd_WorkspaceAggregatesTicketsAcrossRepos(t *testing.T) {
	workspaceRoot := t.TempDir()
	repo = workspaceRoot
	format = "context"
	listFull = false
	listState = "open"
	listWorkspace = true

	if err := os.WriteFile(filepath.Join(workspaceRoot, ".gitmodules"), []byte(`
[submodule "goship"]
	path = goship
	url = git@github.com:example/goship.git
[submodule "control-plane"]
	path = control-plane
	url = git@github.com:example/control-plane.git
`), 0o644); err != nil {
		t.Fatalf("write .gitmodules failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	for _, item := range []struct {
		repoPath string
		id       string
		title    string
	}{
		{repoPath: filepath.Join(workspaceRoot, "goship"), id: "TKT-101", title: "GoShip leaf"},
		{repoPath: filepath.Join(workspaceRoot, "control-plane"), id: "TKT-201", title: "Control plane leaf"},
	} {
		if err := os.MkdirAll(filepath.Join(item.repoPath, ".docket"), 0o755); err != nil {
			t.Fatalf("mkdir .docket failed: %v", err)
		}
		if err := ticket.SaveConfig(item.repoPath, ticket.DefaultConfig()); err != nil {
			t.Fatalf("save config failed for %s: %v", item.repoPath, err)
		}
		s := local.New(item.repoPath)
		if err := s.CreateTicket(ctx, &ticket.Ticket{
			ID: item.id, Seq: 1, Title: item.title, State: ticket.State("todo"), Priority: 1,
			CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{Description: "x"}},
		}); err != nil {
			t.Fatalf("create ticket failed for %s: %v", item.repoPath, err)
		}
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"list", "--workspace", "--format", "context"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("workspace list failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "[control-plane/TKT-201]") || !strings.Contains(got, "[goship/TKT-101]") {
		t.Fatalf("expected aggregated workspace tickets, got:\n%s", got)
	}
}

func TestListCmd_UsesSharedRepoRootWhenInvokedFromWorktree(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "context"
	listFull = false
	listState = "open"

	runGitSession(t, tmpDir, "init")
	runGitSession(t, tmpDir, "config", "user.email", "test@example.com")
	runGitSession(t, tmpDir, "config", "user.name", "Test User")
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	s := local.New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID: "TKT-001", Seq: 1, Title: "Canonical ticket", State: ticket.State("todo"), Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{Description: "x"}},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	runGitSession(t, tmpDir, "add", ".")
	runGitSession(t, tmpDir, "commit", "-m", "seed ticket")

	worktreePath := filepath.Join(tmpDir, "wt", "TKT-001")
	runGitSession(t, tmpDir, "worktree", "add", "-b", "docket/test-list", worktreePath)

	worktreeTicketPath := filepath.Join(worktreePath, ".docket", "tickets", "TKT-001.md")
	raw, err := os.ReadFile(worktreeTicketPath)
	if err != nil {
		t.Fatalf("read worktree ticket: %v", err)
	}
	stale := strings.Replace(string(raw), "state: todo", "state: in-progress", 1)
	if stale == string(raw) {
		t.Fatal("expected worktree ticket state line to be rewritten")
	}
	if err := os.WriteFile(worktreeTicketPath, []byte(stale), 0o644); err != nil {
		t.Fatalf("write stale worktree ticket: %v", err)
	}

	oldRepo := repo
	repo = worktreePath
	t.Cleanup(func() { repo = oldRepo })

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"list", "--format", "context"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list from worktree failed: %v", err)
	}
	if !strings.Contains(out.String(), "[TKT-001] P1 todo") {
		t.Fatalf("expected list to read canonical root state, got:\n%s", out.String())
	}
	if strings.Contains(out.String(), "[TKT-001] P1 in-progress") {
		t.Fatalf("expected stale worktree state to be ignored, got:\n%s", out.String())
	}
}

func TestListCmd_WholeShowsFullHierarchy(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	listFull = false

	prepareFullHierarchyTickets(t, tmpDir)

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"list", "--full"})
	listState = "open"
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list full failed: %v", err)
	}
	if !strings.Contains(out.String(), "TKT-001") || !strings.Contains(out.String(), "↳ TKT-002") {
		t.Fatalf("expected full view to show full hierarchy, got:\n%s", out.String())
	}
}

func TestListCmd_AllAliasShowsFullHierarchy(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	listFull = false

	prepareFullHierarchyTickets(t, tmpDir)

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"list", "--all"})
	listState = "open"
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list alias failed: %v", err)
	}
	if !strings.Contains(out.String(), "TKT-001") || !strings.Contains(out.String(), "↳ TKT-002") {
		t.Fatalf("expected alias view to show full hierarchy, got:\n%s", out.String())
	}
}

func prepareFullHierarchyTickets(t *testing.T, repoDir string) {
	t.Helper()
	s := local.New(repoDir)
	if err := ticket.SaveConfig(repoDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID: "TKT-001", Seq: 1, Title: "Epic", State: ticket.State("todo"), Priority: 1, Labels: []string{"epic"},
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{Description: "x"}},
	}); err != nil {
		t.Fatalf("create epic failed: %v", err)
	}
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID: "TKT-002", Seq: 2, Title: "Child", Parent: "TKT-001", State: ticket.State("todo"), Priority: 2,
		CreatedAt: now.Add(time.Minute), UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{Description: "x"}},
	}); err != nil {
		t.Fatalf("create child failed: %v", err)
	}
}

func TestListCmd_DefaultHidesBlockedBranch(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	listFull = false

	s := local.New(tmpDir)
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config failed: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID: "TKT-099", Seq: 99, Title: "Active blocker", State: ticket.State("in-progress"), Priority: 1,
		CreatedAt: now.Add(-time.Minute), UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{Description: "x"}},
	}); err != nil {
		t.Fatalf("create blocker failed: %v", err)
	}
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID: "TKT-001", Seq: 1, Title: "Blocked epic", State: ticket.State("todo"), Priority: 1, Labels: []string{"epic"}, BlockedBy: []string{"TKT-099"},
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{Description: "x"}},
	}); err != nil {
		t.Fatalf("create epic failed: %v", err)
	}
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID: "TKT-002", Seq: 2, Title: "Blocked child", Parent: "TKT-001", State: ticket.State("todo"), Priority: 2, BlockedBy: []string{"TKT-099"},
		CreatedAt: now.Add(time.Minute), UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{Description: "x"}},
	}); err != nil {
		t.Fatalf("create child failed: %v", err)
	}
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID: "TKT-003", Seq: 3, Title: "Ready", State: ticket.State("todo"), Priority: 3,
		CreatedAt: now.Add(2 * time.Minute), UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{Description: "x"}},
	}); err != nil {
		t.Fatalf("create ready ticket failed: %v", err)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"list"})
	listState = "open"
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(out.String(), "TKT-003") {
		t.Fatalf("expected workable ticket in output, got:\n%s", out.String())
	}
	if strings.Contains(out.String(), "TKT-001") || strings.Contains(out.String(), "TKT-002") {
		t.Fatalf("expected blocked branch hidden from default list, got:\n%s", out.String())
	}
}
