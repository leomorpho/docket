package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/agentrun"
	"github.com/leomorpho/docket/internal/security"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestStartInstruction_Default(t *testing.T) {
	got := startInstruction("TKT-123")
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
		t.Fatalf("instruction should not contain yolo guidance: %q", got)
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
		DefaultState:    "queued",
		DefaultPriority: 10,
		HandoffSections: ticket.DefaultConfig().HandoffSections,
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

func TestSelectNextTicket_LegacyCanonicalMapConfigRemainsWorkable(t *testing.T) {
	tmpDir := t.TempDir()
	s := local.New(tmpDir)
	ctx := context.Background()

	raw := `{
  "counter": 1,
  "backend": "local",
  "states": {
    "backlog": {
      "label": "Backlog",
      "open": true,
      "column": 0,
      "next": ["todo", "in-progress", "archived"],
      "startable": true
    },
    "todo": {
      "label": "To Do",
      "open": true,
      "column": 1,
      "next": ["in-progress", "backlog", "archived"],
      "startable": true
    },
    "in-progress": {
      "label": "In Progress",
      "open": true,
      "column": 2,
      "next": ["in-review", "todo", "archived"]
    },
    "in-review": {
      "label": "In Review",
      "open": true,
      "column": 3,
      "next": ["done", "in-progress", "archived"]
    },
    "done": {
      "label": "Done",
      "open": false,
      "column": 4,
      "next": ["archived", "in-progress"]
    },
    "archived": {
      "label": "Archived",
      "open": false,
      "column": 5,
      "next": ["backlog"]
    }
  },
  "labels": ["bug"],
  "commit_sessions": false
}`
	if err := os.MkdirAll(filepath.Join(tmpDir, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".docket", "config.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := ticket.LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "Actionable backlog ticket",
		State:       "backlog",
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}
	if err := s.SyncIndex(ctx); err != nil {
		t.Fatalf("SyncIndex failed: %v", err)
	}

	got, err := selectNextTicket(ctx, s, cfg)
	if err != nil {
		t.Fatalf("selectNextTicket failed: %v", err)
	}
	if got == nil || got.ID != "TKT-001" {
		t.Fatalf("expected legacy map config to keep backlog ticket workable, got %#v", got)
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

func TestStartRunDelegatesToManagedRunnerForNextTicket(t *testing.T) {
	prev := newRunOrchestrator
	prevStartRun := startRun
	prevStartAuto := startAuto
	prevRunWithReview := runWithReview
	prevRepo := repo
	prevFormat := format
	t.Cleanup(func() {
		newRunOrchestrator = prev
		startRun = prevStartRun
		startAuto = prevStartAuto
		runWithReview = prevRunWithReview
		repo = prevRepo
		format = prevFormat
	})

	tmpRepo := t.TempDir()
	repo = tmpRepo
	format = "human"
	startRun = false
	startAuto = false
	runWithReview = false

	if err := ticket.SaveConfig(tmpRepo, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	s := local.New(tmpRepo)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-501",
		Seq:         501,
		Title:       "Managed start run",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	var gotTicket string
	var gotReview bool
	newRunOrchestrator = func(repoRoot string, enableReview bool) agentrun.Orchestrator {
		gotReview = enableReview
		return stubRunOrchestrator{
			runTicket: func(ctx context.Context, ticketID string) (agentrun.TicketRunSummary, error) {
				gotTicket = ticketID
				return agentrun.TicketRunSummary{TicketID: ticketID, Status: agentrun.StatusDone, Reason: "validated and advanced"}, nil
			},
		}
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"start", "--run"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("start --run failed: %v\n%s", err, out.String())
	}

	if gotTicket != "TKT-501" {
		t.Fatalf("expected start --run to choose TKT-501, got %q", gotTicket)
	}
	if gotReview {
		t.Fatalf("expected review disabled by default")
	}
	if got := out.String(); !strings.Contains(got, "TKT-501: done") {
		t.Fatalf("unexpected output: %s", got)
	}
}

func TestStartRunAutoDelegatesToSerialRunner(t *testing.T) {
	prev := newRunOrchestrator
	prevStartRun := startRun
	prevStartAuto := startAuto
	prevRunWithReview := runWithReview
	prevRepo := repo
	prevFormat := format
	t.Cleanup(func() {
		newRunOrchestrator = prev
		startRun = prevStartRun
		startAuto = prevStartAuto
		runWithReview = prevRunWithReview
		repo = prevRepo
		format = prevFormat
	})

	repo = t.TempDir()
	format = "human"
	startRun = false
	startAuto = false
	runWithReview = false
	if err := ticket.SaveConfig(repo, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	var runNextCalled bool
	newRunOrchestrator = func(repoRoot string, enableReview bool) agentrun.Orchestrator {
		if !enableReview {
			// keep branch explicit; review not needed for this test
		}
		return stubRunOrchestrator{
			runNext: func(ctx context.Context) (agentrun.CycleSummary, error) {
				runNextCalled = true
				return agentrun.CycleSummary{
					Runs: []agentrun.TicketRunSummary{
						{TicketID: "TKT-601", Status: agentrun.StatusDone},
						{TicketID: "TKT-602", Status: agentrun.StatusFailed, Reason: "blocked"},
					},
					StopReason: "blocked",
				}, nil
			},
		}
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"start", "--run", "--auto"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("start --run --auto failed: %v\n%s", err, out.String())
	}

	if !runNextCalled {
		t.Fatalf("expected start --run --auto to delegate to RunNext")
	}
	if got := out.String(); !strings.Contains(got, "TKT-601: done") || !strings.Contains(got, "Stopped: blocked") {
		t.Fatalf("unexpected output: %s", got)
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
	if !strings.Contains(b.String(), "=== Docket Intro ===") {
		t.Fatalf("expected intro block on empty workable-set path, got:\n%s", b.String())
	}
	if !strings.Contains(b.String(), "docket skill list --format json") {
		t.Fatalf("expected skill reminder in empty workable-set intro, got:\n%s", b.String())
	}
}
