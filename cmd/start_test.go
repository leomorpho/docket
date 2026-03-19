package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/security"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestStartInstruction_Default(t *testing.T) {
	got := startInstruction("TKT-123", false)
	wantContains := []string{
		"Work only ticket TKT-123",
		"test-driven development",
		"write or update tests first",
		"smallest passing change",
		"`Ticket: <TKT-NNN>` trailer",
		"`Ticket: TKT-123`",
	}
	for _, want := range wantContains {
		if !strings.Contains(got, want) {
			t.Fatalf("default instruction missing %q in %q", want, got)
		}
	}
	if strings.Contains(strings.ToLower(got), "yolo mode") {
		t.Fatalf("default instruction should not contain yolo guidance: %q", got)
	}
}

func TestStartInstruction_Yolo(t *testing.T) {
	got := startInstruction("TKT-123", true)
	wantContains := []string{
		"YOLO mode",
		"test-driven development",
		"`docket list --state open --format context`",
		"underspecified or non-viable",
		"update the ticket details/AC",
		"`Ticket: <TKT-NNN>` trailer",
		"identify the next best ticket and repeat",
		"Stop when no viable tickets remain",
	}
	for _, want := range wantContains {
		if !strings.Contains(got, want) {
			t.Fatalf("yolo instruction missing %q in %q", want, got)
		}
	}
}

func TestSelectNextTicket(t *testing.T) {
	tmpDir := t.TempDir()
	s := local.New(tmpDir)
	ctx := context.Background()

	// 1. Setup config
	cfg := ticket.DefaultConfig()
	// Add in-progress to backlog.next so transitions work in selectNextTicket
	backlog := cfg.States["backlog"]
	backlog.Next = append(backlog.Next, "in-progress")
	cfg.States["backlog"] = backlog

	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// 2. Create some tickets
	// T1: priority 10, backlog
	// T2: priority 1, backlog (should be picked)
	// T3: priority 1, backlog, blocked (should be skipped)
	// T4: in-progress (should be skipped)
	// T5: in-review (should be skipped)

	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "T1", State: "backlog", Priority: 10})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-002", Seq: 2, Title: "T2", State: "backlog", Priority: 1})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-003", Seq: 3, Title: "T3", State: "backlog", Priority: 1, BlockedBy: []string{"TKT-001"}})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-004", Seq: 4, Title: "T4", State: "in-progress", Priority: 1})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-005", Seq: 5, Title: "T5", State: "in-review", Priority: 1})

	if err := s.SyncIndex(ctx); err != nil {
		t.Fatalf("SyncIndex failed: %v", err)
	}

	// 3. Select next
	got, err := selectNextTicket(ctx, s, cfg)
	if err != nil {
		t.Fatalf("selectNextTicket failed: %v", err)
	}

	if got == nil {
		t.Fatal("expected a ticket, got nil")
	}
	if got.ID != "TKT-002" {
		t.Errorf("expected TKT-002, got %s", got.ID)
	}
}

func TestSelectNextTicket_HonorsConfigForReviewBlockers(t *testing.T) {
	tmpDir := t.TempDir()
	s := local.New(tmpDir)
	ctx := context.Background()

	cfg := ticket.DefaultConfig()
	backlog := cfg.States["backlog"]
	backlog.Next = append(backlog.Next, "in-progress")
	cfg.States["backlog"] = backlog
	review := cfg.States["in-review"]
	review.BlocksDependents = false
	cfg.States["in-review"] = review

	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "Reviewed blocker", State: "in-review", Priority: 1})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-002", Seq: 2, Title: "Dependent", State: "backlog", Priority: 1, BlockedBy: []string{"TKT-001"}})

	if err := s.SyncIndex(ctx); err != nil {
		t.Fatalf("SyncIndex failed: %v", err)
	}

	got, err := selectNextTicket(ctx, s, cfg)
	if err != nil {
		t.Fatalf("selectNextTicket failed: %v", err)
	}
	if got == nil || got.ID != "TKT-002" {
		t.Fatalf("expected dependent ticket to become selectable, got %#v", got)
	}
}

func TestSelectNextTicket_SkipsEpics(t *testing.T) {
	tmpDir := t.TempDir()
	s := local.New(tmpDir)
	ctx := context.Background()

	cfg := ticket.DefaultConfig()
	backlog := cfg.States["backlog"]
	backlog.Next = append(backlog.Next, "in-progress")
	cfg.States["backlog"] = backlog
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-010",
		Seq:         10,
		Title:       "[Epic] Umbrella",
		State:       "backlog",
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
		Labels:      []string{"epic"},
	}); err != nil {
		t.Fatalf("CreateTicket epic failed: %v", err)
	}
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-011",
		Seq:         11,
		Title:       "Actionable",
		State:       "backlog",
		Priority:    2,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
	}); err != nil {
		t.Fatalf("CreateTicket actionable failed: %v", err)
	}
	if err := s.SyncIndex(ctx); err != nil {
		t.Fatalf("SyncIndex failed: %v", err)
	}

	got, err := selectNextTicket(ctx, s, cfg)
	if err != nil {
		t.Fatalf("selectNextTicket failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected a ticket, got nil")
	}
	if got.ID != "TKT-011" {
		t.Fatalf("expected non-epic ticket TKT-011, got %s", got.ID)
	}
}

func TestSelectNextTicket_UsesStartableStatesFromConfig(t *testing.T) {
	tmpDir := t.TempDir()
	s := local.New(tmpDir)
	ctx := context.Background()

	cfg := ticket.DefaultConfig()
	delete(cfg.States, "backlog")
	delete(cfg.States, "todo")
	cfg.States["queued"] = ticket.StateConfig{
		Label:            "Queued",
		Open:             true,
		Column:           0,
		Next:             []string{"in-progress"},
		Roles:            []string{"intake"},
		Startable:        true,
		BlocksDependents: true,
	}
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "Queued work",
		State:       "queued",
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	got, err := selectNextTicket(ctx, s, cfg)
	if err != nil {
		t.Fatalf("selectNextTicket failed: %v", err)
	}
	if got == nil || got.ID != "TKT-001" {
		t.Fatalf("expected queued ticket to be selected, got %#v", got)
	}
}

func TestStartCmd_AllowsUnsecuredManagedRun(t *testing.T) {
	tmpRepo := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	tmpUserHome := filepath.Join(t.TempDir(), "home")
	t.Setenv("DOCKET_HOME", tmpHome)
	t.Setenv("DOCKET_AGENT_ID", "test-agent")
	t.Setenv("HOME", tmpUserHome)
	docketHome = ""
	repo = tmpRepo
	format = "human"

	runGitSession(t, tmpRepo, "init")
	runGitSession(t, tmpRepo, "config", "user.email", "test@example.com")
	runGitSession(t, tmpRepo, "config", "user.name", "Test User")
	seedPath := filepath.Join(tmpRepo, "seed.txt")
	if err := os.WriteFile(seedPath, []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed file failed: %v", err)
	}
	runGitSession(t, tmpRepo, "add", ".")
	runGitSession(t, tmpRepo, "commit", "-m", "chore: seed")

	if err := ticket.SaveConfig(tmpRepo, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	s := local.New(tmpRepo)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "Implement feature",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test-agent",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"start"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("start failed without secure setup: %v", err)
	}

	got, err := s.GetTicket(ctx, "TKT-001")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if got.State != ticket.State("in-progress") {
		t.Fatalf("expected in-progress state, got %s", got.State)
	}
	if !strings.Contains(b.String(), "Runtime policy: unsecured") {
		t.Fatalf("expected unsecured runtime note in output, got: %s", b.String())
	}

	ns := security.NewRepoNamespaceStore(tmpHome)
	run, ok, err := ns.GetRunManifest(tmpRepo, "TKT-001")
	if err != nil || !ok {
		t.Fatalf("expected run manifest, ok=%v err=%v", ok, err)
	}
	if run.WorkflowHash != "" {
		t.Fatalf("expected unsecured run to record empty workflow hash, got %q", run.WorkflowHash)
	}
	if run.Actor != "agent:test-agent" {
		t.Fatalf("expected agent actor, got %q", run.Actor)
	}
	if run.WorktreePath == "" || run.WorktreePath == tmpRepo {
		t.Fatalf("expected dedicated worktree path, got %q", run.WorktreePath)
	}
}

func TestStartAutoPreservesReviewTransitionBehavior(t *testing.T) {
	tmpRepo := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	tmpUserHome := filepath.Join(t.TempDir(), "home")
	t.Setenv("DOCKET_HOME", tmpHome)
	t.Setenv("DOCKET_AGENT_ID", "test-agent")
	t.Setenv("HOME", tmpUserHome)
	docketHome = ""
	repo = tmpRepo
	format = "human"

	runGitSession(t, tmpRepo, "init")
	runGitSession(t, tmpRepo, "config", "user.email", "test@example.com")
	runGitSession(t, tmpRepo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(tmpRepo, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed file failed: %v", err)
	}
	runGitSession(t, tmpRepo, "add", ".")
	runGitSession(t, tmpRepo, "commit", "-m", "chore: seed")

	if err := ticket.SaveConfig(tmpRepo, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	s := local.New(tmpRepo)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-777",
		Seq:         777,
		Title:       "Start auto smoke",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test-agent",
		Description: "D",
		Handoff:     "**Current state:** todo\n**Decisions made:** none\n**Files touched:** n/a\n**Remaining work:** start\n**AC status:** pending",
		AC: []ticket.AcceptanceCriterion{
			{Description: "A1", Done: true, Evidence: "done"},
			{Description: "A2", Done: true, Evidence: "done"},
		},
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"start", "--auto"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("start --auto failed: %v", err)
	}

	got, err := s.GetTicket(context.Background(), "TKT-777")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if got.State != ticket.State("in-progress") {
		t.Fatalf("expected start to preserve in-progress transition behavior, got %s", got.State)
	}
}

func TestStartCmd_EmptyWorkableSetShowsStartableStates(t *testing.T) {
	tmpRepo := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	docketHome = ""
	repo = tmpRepo
	format = "human"

	cfg := ticket.DefaultConfig()
	for name, state := range cfg.States {
		state.Startable = false
		cfg.States[name] = state
	}
	if err := ticket.SaveConfig(tmpRepo, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"start"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if !strings.Contains(b.String(), "No workable tickets found.") {
		t.Fatalf("expected empty workable-set message, got:\n%s", b.String())
	}
	if !strings.Contains(b.String(), "Startable states in current config: none configured.") {
		t.Fatalf("expected startable-state summary, got:\n%s", b.String())
	}
}
