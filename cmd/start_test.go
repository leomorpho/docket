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
	runruntime "github.com/leomorpho/docket/internal/agentrun/runtime"
	"github.com/leomorpho/docket/internal/claim"
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
	// Extend the default workflow example so backlog can transition directly
	// into an active-role state during this test setup.
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
	// T4: active-role state in default workflow example (should be skipped)
	// T5: review-role state in default workflow example (should be skipped)

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

func TestSelectNextTicket_ReleasesStaleClaimForStartableTicket(t *testing.T) {
	tmpDir := t.TempDir()
	s := local.New(tmpDir)
	ctx := context.Background()
	runGitSession(t, tmpDir, "init")

	cfg := ticket.DefaultConfig()
	backlog := cfg.States["backlog"]
	backlog.Next = append(backlog.Next, "in-progress")
	cfg.States["backlog"] = backlog
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-030",
		Seq:         30,
		Title:       "Stale claim leaf",
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
	if err := claim.Claim(tmpDir, "TKT-030", filepath.Join(tmpDir, "wt-030"), "human:test"); err != nil {
		t.Fatalf("Claim() failed: %v", err)
	}

	got, err := selectNextTicket(ctx, s, cfg)
	if err != nil {
		t.Fatalf("selectNextTicket failed: %v", err)
	}
	if got == nil || got.ID != "TKT-030" {
		t.Fatalf("expected stale claimed ticket to become selectable, got %#v", got)
	}
	cl, err := claim.GetClaim(tmpDir, "TKT-030")
	if err != nil {
		t.Fatalf("GetClaim() failed: %v", err)
	}
	if cl != nil {
		t.Fatalf("expected stale claim to be released, got %#v", cl)
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

func TestSelectNextTicket_SkipsCoordinationTickets(t *testing.T) {
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
		ID:          "TKT-020",
		Seq:         20,
		Title:       "Program: Framework Beauty",
		State:       "backlog",
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
		Labels:      []string{"program", "topo:coordination"},
	}); err != nil {
		t.Fatalf("CreateTicket program failed: %v", err)
	}
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-021",
		Seq:         21,
		Title:       "Epic: Starter Quality",
		State:       "backlog",
		Priority:    2,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
		Labels:      []string{"topo:coordination"},
	}); err != nil {
		t.Fatalf("CreateTicket epic failed: %v", err)
	}
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-022",
		Seq:         22,
		Title:       "Actionable leaf",
		State:       "backlog",
		Priority:    3,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
		Labels:      []string{"topo:leaf"},
	}); err != nil {
		t.Fatalf("CreateTicket leaf failed: %v", err)
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
	if got.ID != "TKT-022" {
		t.Fatalf("expected non-coordination ticket TKT-022, got %s", got.ID)
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

func TestSelectNextTicket_SkipsClaimedTickets(t *testing.T) {
	tmpDir := t.TempDir()
	runGitSession(t, tmpDir, "init")
	s := local.New(tmpDir)
	ctx := context.Background()

	cfg := ticket.DefaultConfig()
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	for _, tc := range []struct {
		id       string
		priority int
	}{
		{id: "TKT-001", priority: 1},
		{id: "TKT-002", priority: 2},
	} {
		if err := s.CreateTicket(ctx, &ticket.Ticket{
			ID:          tc.id,
			Seq:         tc.priority,
			Title:       tc.id,
			State:       "todo",
			Priority:    tc.priority,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "D",
			AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
		}); err != nil {
			t.Fatalf("CreateTicket(%s) failed: %v", tc.id, err)
		}
	}
	if err := s.SyncIndex(ctx); err != nil {
		t.Fatalf("SyncIndex failed: %v", err)
	}
	if err := claim.Claim(tmpDir, "TKT-001", filepath.Join(tmpDir, "wt-001"), "human:test"); err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	got, err := selectNextTicket(ctx, s, cfg)
	if err != nil {
		t.Fatalf("selectNextTicket failed: %v", err)
	}
	if got == nil || got.ID != "TKT-002" {
		t.Fatalf("expected claimed ticket to be skipped, got %#v", got)
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
	if !strings.Contains(b.String(), "Security enforcement: warning-only") {
		t.Fatalf("expected warning-only enforcement note in output, got: %s", b.String())
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

func TestStartCmd_ReportsEnabledSecurityEnforcement(t *testing.T) {
	h := newFakeRepoHarness(t)
	cfg, err := ticket.LoadConfig(h.repo)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	cfg.SecurityEnforcement = true
	if err := ticket.SaveConfig(h.repo, cfg); err != nil {
		t.Fatalf("save config failed: %v", err)
	}
	h.seedTicket("TKT-810", 810, ticket.State("todo"), []ticket.AcceptanceCriterion{{Description: "ac"}})

	out, err := h.run("start")
	if err != nil {
		t.Fatalf("start failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Security enforcement: enabled") {
		t.Fatalf("expected enabled enforcement note in output, got:\n%s", out)
	}
}

func TestStartRunDelegatesToManagedRunnerForNextTicket(t *testing.T) {
	prev := newRunOrchestrator
	prevStartRun := startRun
	prevStartAuto := startAuto
	prevNoReview := runDisableReview
	prevRepo := repo
	prevFormat := format
	t.Cleanup(func() {
		newRunOrchestrator = prev
		startRun = prevStartRun
		startAuto = prevStartAuto
		runDisableReview = prevNoReview
		repo = prevRepo
		format = prevFormat
	})

	tmpRepo := t.TempDir()
	repo = tmpRepo
	format = "human"
	startRun = false
	startAuto = false
	runDisableReview = false

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
	if !gotReview {
		t.Fatalf("expected review enabled by default")
	}
	if got := out.String(); !strings.Contains(got, "TKT-501: done") {
		t.Fatalf("unexpected output: %s", got)
	}
}

func TestStartRunAutoDelegatesToSerialRunner(t *testing.T) {
	prev := newRunOrchestrator
	prevStartRun := startRun
	prevStartAuto := startAuto
	prevNoReview := runDisableReview
	prevRepo := repo
	prevFormat := format
	t.Cleanup(func() {
		newRunOrchestrator = prev
		startRun = prevStartRun
		startAuto = prevStartAuto
		runDisableReview = prevNoReview
		repo = prevRepo
		format = prevFormat
	})

	repo = t.TempDir()
	format = "human"
	startRun = false
	startAuto = false
	runDisableReview = false
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

func TestStartRunNoReviewDisablesReviewerLoop(t *testing.T) {
	prev := newRunOrchestrator
	prevStartRun := startRun
	prevStartAuto := startAuto
	prevNoReview := runDisableReview
	prevRepo := repo
	prevFormat := format
	t.Cleanup(func() {
		newRunOrchestrator = prev
		startRun = prevStartRun
		startAuto = prevStartAuto
		runDisableReview = prevNoReview
		repo = prevRepo
		format = prevFormat
	})

	tmpRepo := t.TempDir()
	repo = tmpRepo
	format = "human"
	startRun = false
	startAuto = false
	runDisableReview = false

	if err := ticket.SaveConfig(tmpRepo, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	s := local.New(tmpRepo)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-502",
		Seq:         502,
		Title:       "Managed start no review",
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

	var gotReview bool
	newRunOrchestrator = func(repoRoot string, enableReview bool) agentrun.Orchestrator {
		gotReview = enableReview
		return stubRunOrchestrator{
			runTicket: func(ctx context.Context, ticketID string) (agentrun.TicketRunSummary, error) {
				return agentrun.TicketRunSummary{TicketID: ticketID, Status: agentrun.StatusDone}, nil
			},
		}
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"start", "--run", "--no-review"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("start --run --no-review failed: %v\n%s", err, out.String())
	}
	if gotReview {
		t.Fatalf("expected --no-review to disable reviewer loop")
	}
}

func TestStartRunAutoStreamsLiveTranscriptFromRuntimeFiles(t *testing.T) {
	prev := newRunOrchestrator
	prevStartRun := startRun
	prevStartAuto := startAuto
	prevNoReview := runDisableReview
	prevRepo := repo
	prevFormat := format
	t.Cleanup(func() {
		newRunOrchestrator = prev
		startRun = prevStartRun
		startAuto = prevStartAuto
		runDisableReview = prevNoReview
		repo = prevRepo
		format = prevFormat
	})

	repoRoot := t.TempDir()
	repo = repoRoot
	format = "human"
	startRun = false
	startAuto = false
	runDisableReview = false
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	store := runruntime.New(repoRoot)

	newRunOrchestrator = func(repoRoot string, enableReview bool) agentrun.Orchestrator {
		return stubRunOrchestrator{
			runNext: func(ctx context.Context) (agentrun.CycleSummary, error) {
				record := agentrun.RunRecord{
					TicketID:     "TKT-700",
					Role:         agentrun.RoleImplementer,
					RepoRoot:     repoRoot,
					WorktreePath: repoRoot,
					Branch:       "docket/TKT-700",
					SessionID:    "session-700",
				}
				if err := store.Init(record, "prompt", 10*time.Minute); err != nil {
					t.Fatalf("Init() error = %v", err)
				}
				time.Sleep(300 * time.Millisecond)
				if err := store.AppendTranscript("TKT-700", runruntime.TranscriptEntry{At: time.Now().UTC().Format(time.RFC3339Nano), Text: "PLAN ticket=TKT-700 steps=2"}); err != nil {
					t.Fatalf("AppendTranscript() error = %v", err)
				}
				time.Sleep(300 * time.Millisecond)
				if err := store.AppendTranscript("TKT-700", runruntime.TranscriptEntry{At: time.Now().UTC().Format(time.RFC3339Nano), Text: "STEP ticket=TKT-700 index=1 status=done title=\"inspect scope\""}); err != nil {
					t.Fatalf("AppendTranscript() error = %v", err)
				}
				return agentrun.CycleSummary{
					Runs: []agentrun.TicketRunSummary{{TicketID: "TKT-700", Status: agentrun.StatusDone}},
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
	got := out.String()
	for _, want := range []string{
		"[TKT-700] session=session-700 active=true",
		"[TKT-700] PLAN ticket=TKT-700 steps=2",
		"[TKT-700] STEP ticket=TKT-700 index=1 status=done title=\"inspect scope\"",
		"TKT-700: done",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in output, got:\n%s", want, got)
		}
	}
}

func TestStartAutoImpliesManagedRun(t *testing.T) {
	prev := newRunOrchestrator
	prevStartRun := startRun
	prevStartAuto := startAuto
	prevNoReview := runDisableReview
	prevRepo := repo
	prevFormat := format
	t.Cleanup(func() {
		newRunOrchestrator = prev
		startRun = prevStartRun
		startAuto = prevStartAuto
		runDisableReview = prevNoReview
		repo = prevRepo
		format = prevFormat
	})

	repo = t.TempDir()
	format = "human"
	startRun = false
	startAuto = false
	runDisableReview = false
	if err := ticket.SaveConfig(repo, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	var runNextCalled bool
	newRunOrchestrator = func(repoRoot string, enableReview bool) agentrun.Orchestrator {
		return stubRunOrchestrator{
			runNext: func(ctx context.Context) (agentrun.CycleSummary, error) {
				runNextCalled = true
				return agentrun.CycleSummary{
					Runs: []agentrun.TicketRunSummary{{TicketID: "TKT-777", Status: agentrun.StatusDone}},
				}, nil
			},
		}
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"start", "--auto"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("start --auto failed: %v\n%s", err, out.String())
	}
	if !runNextCalled {
		t.Fatalf("expected start --auto to imply managed run cycle")
	}
	if got := out.String(); !strings.Contains(got, "TKT-777: done") {
		t.Fatalf("unexpected output: %s", got)
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

func TestStartCmd_EmptyWorkableSetExplainsBlockedBacklog(t *testing.T) {
	tmpRepo := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	docketHome = ""
	repo = tmpRepo
	format = "human"

	if err := ticket.SaveConfig(tmpRepo, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	s := local.New(tmpRepo)
	now := time.Now().UTC().Truncate(time.Second)
	for _, tk := range []*ticket.Ticket{
		{
			ID:          "TKT-001",
			Seq:         1,
			Title:       "Active blocker",
			State:       ticket.State("in-progress"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "desc",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
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
			CreatedBy:   "human:test",
			Description: "desc",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		},
	} {
		if err := s.CreateTicket(context.Background(), tk); err != nil {
			t.Fatalf("CreateTicket failed: %v", err)
		}
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"start"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if !strings.Contains(b.String(), "Backlog warning: none are runnable right now") {
		t.Fatalf("expected blocked backlog warning, got:\n%s", b.String())
	}
	if !strings.Contains(b.String(), "Top unresolved blockers: TKT-001 x1") {
		t.Fatalf("expected blocker detail, got:\n%s", b.String())
	}
}
