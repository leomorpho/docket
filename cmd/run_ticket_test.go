package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/agentrun"
	runruntime "github.com/leomorpho/docket/internal/agentrun/runtime"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/leomorpho/docket/internal/workspace"
	"github.com/spf13/cobra"
)

type stubRunOrchestrator struct {
	runTicket func(ctx context.Context, ticketID string) (agentrun.TicketRunSummary, error)
	runNext   func(ctx context.Context) (agentrun.CycleSummary, error)
	resume    func(ctx context.Context, ticketID string) (agentrun.TicketRunSummary, error)
	ping      func(ctx context.Context, ticketID string) (agentrun.PingSummary, error)
}

func (s stubRunOrchestrator) RunTicket(ctx context.Context, ticketID string) (agentrun.TicketRunSummary, error) {
	if s.runTicket == nil {
		return agentrun.TicketRunSummary{}, nil
	}
	return s.runTicket(ctx, ticketID)
}

func (s stubRunOrchestrator) RunNext(ctx context.Context) (agentrun.CycleSummary, error) {
	if s.runNext == nil {
		return agentrun.CycleSummary{}, nil
	}
	return s.runNext(ctx)
}

func (s stubRunOrchestrator) ResumeTicket(ctx context.Context, ticketID string) (agentrun.TicketRunSummary, error) {
	if s.resume == nil {
		return agentrun.TicketRunSummary{}, nil
	}
	return s.resume(ctx, ticketID)
}

func (s stubRunOrchestrator) PingTicket(ctx context.Context, ticketID string) (agentrun.PingSummary, error) {
	if s.ping == nil {
		return agentrun.PingSummary{}, nil
	}
	return s.ping(ctx, ticketID)
}

func TestRunTicketCmdRendersJSONSummary(t *testing.T) {
	prev := newRunOrchestrator
	prevNoReview := runDisableReview
	t.Cleanup(func() {
		newRunOrchestrator = prev
		runDisableReview = prevNoReview
	})
	runDisableReview = false
	newRunOrchestrator = func(repoRoot string, enableReview bool) agentrun.Orchestrator {
		if !enableReview {
			t.Fatalf("expected review enabled by default")
		}
		return stubRunOrchestrator{
			runTicket: func(ctx context.Context, ticketID string) (agentrun.TicketRunSummary, error) {
				return agentrun.TicketRunSummary{TicketID: ticketID, Status: agentrun.StatusDone, Reason: "validated and advanced"}, nil
			},
		}
	}

	var out bytes.Buffer
	repo = t.TempDir()
	format = "json"
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"run-ticket", "TKT-376", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("run-ticket failed: %v\n%s", err, out.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, out.String())
	}
	if payload["ticket_id"] != "TKT-376" || payload["status"] != "done" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestRunNextCmdRendersHumanSummary(t *testing.T) {
	prev := newRunOrchestrator
	prevNoReview := runDisableReview
	t.Cleanup(func() {
		newRunOrchestrator = prev
		runDisableReview = prevNoReview
	})
	runDisableReview = false
	newRunOrchestrator = func(repoRoot string, enableReview bool) agentrun.Orchestrator {
		if !enableReview {
			t.Fatalf("expected review enabled by default")
		}
		return stubRunOrchestrator{
			runNext: func(ctx context.Context) (agentrun.CycleSummary, error) {
				return agentrun.CycleSummary{
					Runs: []agentrun.TicketRunSummary{
						{TicketID: "TKT-376", Status: agentrun.StatusDone},
						{TicketID: "TKT-375", Status: agentrun.StatusFailed, Reason: "review changes required"},
					},
					StopReason: "review changes required",
				}, nil
			},
		}
	}

	var out bytes.Buffer
	repo = t.TempDir()
	format = "human"
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"run-next"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("run-next failed: %v\n%s", err, out.String())
	}

	if got := out.String(); !bytes.Contains([]byte(got), []byte("TKT-376: done")) || !bytes.Contains([]byte(got), []byte("Stopped: review changes required")) {
		t.Fatalf("unexpected output: %s", got)
	}
}

func TestRunTicketCmdNoReviewDisablesReviewerLoop(t *testing.T) {
	prev := newRunOrchestrator
	prevNoReview := runDisableReview
	t.Cleanup(func() {
		newRunOrchestrator = prev
		runDisableReview = prevNoReview
	})
	runDisableReview = false

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
	repo = t.TempDir()
	format = "human"
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"run-ticket", "TKT-376", "--no-review"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("run-ticket --no-review failed: %v\n%s", err, out.String())
	}
	if gotReview {
		t.Fatalf("expected --no-review to disable reviewer loop")
	}
}

func TestRunNextCmdStreamsLiveTranscriptFromRuntimeFiles(t *testing.T) {
	prev := newRunOrchestrator
	prevNoReview := runDisableReview
	prevRepo := repo
	prevFormat := format
	t.Cleanup(func() {
		newRunOrchestrator = prev
		runDisableReview = prevNoReview
		repo = prevRepo
		format = prevFormat
	})

	repoRoot := t.TempDir()
	repo = repoRoot
	format = "human"
	runDisableReview = false
	store := runruntime.New(repoRoot)

	newRunOrchestrator = func(repoRoot string, enableReview bool) agentrun.Orchestrator {
		return stubRunOrchestrator{
			runNext: func(ctx context.Context) (agentrun.CycleSummary, error) {
				record := agentrun.RunRecord{
					TicketID:     "TKT-376",
					Role:         agentrun.RoleImplementer,
					RepoRoot:     repoRoot,
					WorktreePath: repoRoot,
					Branch:       "docket/TKT-376",
					SessionID:    "session-376",
				}
				if err := store.Init(record, "prompt", 10*time.Minute); err != nil {
					t.Fatalf("Init() error = %v", err)
				}
				time.Sleep(300 * time.Millisecond)
				if err := store.AppendTranscript("TKT-376", runruntime.TranscriptEntry{At: time.Now().UTC().Format(time.RFC3339Nano), Text: "PLAN ticket=TKT-376 steps=3"}); err != nil {
					t.Fatalf("AppendTranscript() error = %v", err)
				}
				time.Sleep(300 * time.Millisecond)
				if err := store.AppendTranscript("TKT-376", runruntime.TranscriptEntry{At: time.Now().UTC().Format(time.RFC3339Nano), Text: "STEP ticket=TKT-376 index=1 status=in_progress title=\"inspect repo\""}); err != nil {
					t.Fatalf("AppendTranscript() error = %v", err)
				}
				return agentrun.CycleSummary{
					Runs: []agentrun.TicketRunSummary{{TicketID: "TKT-376", Status: agentrun.StatusDone}},
				}, nil
			},
		}
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"run-next"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("run-next failed: %v\n%s", err, out.String())
	}
	got := out.String()
	for _, want := range []string{
		"[TKT-376] session=session-376 active=true",
		"[TKT-376] PLAN ticket=TKT-376 steps=3",
		"[TKT-376] STEP ticket=TKT-376 index=1 status=in_progress title=\"inspect repo\"",
		"TKT-376: done",
	} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("expected %q in output, got:\n%s", want, got)
		}
	}
}

func TestRunNextCmdIgnoresTranscriptHistoryThatPredatesTheCommand(t *testing.T) {
	prev := newRunOrchestrator
	prevNoReview := runDisableReview
	prevRepo := repo
	prevFormat := format
	t.Cleanup(func() {
		newRunOrchestrator = prev
		runDisableReview = prevNoReview
		repo = prevRepo
		format = prevFormat
	})

	repoRoot := t.TempDir()
	repo = repoRoot
	format = "human"
	runDisableReview = false
	store := runruntime.New(repoRoot)
	record := agentrun.RunRecord{
		TicketID:     "TKT-377",
		Role:         agentrun.RoleImplementer,
		RepoRoot:     repoRoot,
		WorktreePath: repoRoot,
		Branch:       "docket/TKT-377",
		SessionID:    "session-old",
	}
	if err := store.Init(record, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.AppendTranscript("TKT-377", runruntime.TranscriptEntry{At: time.Now().UTC().Format(time.RFC3339Nano), Text: "PLAN ticket=TKT-377 steps=9"}); err != nil {
		t.Fatalf("AppendTranscript() error = %v", err)
	}

	newRunOrchestrator = func(repoRoot string, enableReview bool) agentrun.Orchestrator {
		return stubRunOrchestrator{
			runNext: func(ctx context.Context) (agentrun.CycleSummary, error) {
				time.Sleep(300 * time.Millisecond)
				if err := store.AppendTranscript("TKT-377", runruntime.TranscriptEntry{At: time.Now().UTC().Format(time.RFC3339Nano), Text: "STEP ticket=TKT-377 index=1 status=done title=\"fresh line\""}); err != nil {
					t.Fatalf("AppendTranscript() error = %v", err)
				}
				return agentrun.CycleSummary{
					Runs: []agentrun.TicketRunSummary{{TicketID: "TKT-377", Status: agentrun.StatusDone}},
				}, nil
			},
		}
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"run-next"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("run-next failed: %v\n%s", err, out.String())
	}
	got := out.String()
	if bytes.Contains([]byte(got), []byte("PLAN ticket=TKT-377 steps=9")) {
		t.Fatalf("expected stale transcript to be ignored, got:\n%s", got)
	}
	if !bytes.Contains([]byte(got), []byte("STEP ticket=TKT-377 index=1 status=done title=\"fresh line\"")) {
		t.Fatalf("expected fresh transcript line in output, got:\n%s", got)
	}
}

func TestRunStatusCmdRendersHumanSummaryFromActiveRuntimeFiles(t *testing.T) {
	repoRoot := t.TempDir()
	store := runruntime.New(repoRoot)
	if err := store.WriteStatus(runruntime.StatusSnapshot{
		TicketID:            "TKT-376",
		Active:              true,
		Hung:                true,
		PlannedSteps:        8,
		CurrentStep:         3,
		CurrentStepTitle:    "write failing test",
		CurrentPhase:        "testing",
		LastVisibleText:     "STEP ticket=TKT-376 index=3 status=in_progress title=\"write failing test\"",
		LastEventAt:         time.Now().UTC().Format(time.RFC3339Nano),
		SessionMessageCount: 7,
		HealthCheckCount:    2,
		LastIntervention:    "ping",
	}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}

	var out bytes.Buffer
	repo = repoRoot
	format = "human"
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"run-status", "TKT-376"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("run-status failed: %v\n%s", err, out.String())
	}
	got := out.String()
	if !bytes.Contains([]byte(got), []byte("hung=true")) || !bytes.Contains([]byte(got), []byte("write failing test")) {
		t.Fatalf("unexpected output: %s", got)
	}
	if !bytes.Contains([]byte(got), []byte("Session messages: 7")) {
		t.Fatalf("expected session message count in output, got: %s", got)
	}
	if !bytes.Contains([]byte(got), []byte("Health checks: 2")) || !bytes.Contains([]byte(got), []byte("Last intervention: ping")) {
		t.Fatalf("expected health check summary in output, got: %s", got)
	}
}

func TestRunStatusCmdRendersRuntimeWarning(t *testing.T) {
	repoRoot := t.TempDir()
	store := runruntime.New(repoRoot)
	record := agentrun.RunRecord{TicketID: "TKT-377", Role: agentrun.RoleImplementer, RepoRoot: repoRoot, WorktreePath: repoRoot, Branch: "docket/TKT-377", SessionID: "session-377"}
	if err := store.Init(record, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.WriteStatus(runruntime.StatusSnapshot{
		TicketID: "TKT-377",
		Warning:  "optional MCP server mcp.instantdb.com rejected authentication; continuing without it",
	}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}

	var out bytes.Buffer
	repo = repoRoot
	format = "human"
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"run-status", "TKT-377"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("run-status failed: %v\n%s", err, out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("Warning: optional MCP server mcp.instantdb.com rejected authentication; continuing without it")) {
		t.Fatalf("expected runtime warning in output, got: %s", out.String())
	}
}

func TestTuiRunLogCmdRendersVisibleTranscript(t *testing.T) {
	originalLocal := time.Local
	time.Local = time.FixedZone("PDT", -7*60*60)
	defer func() { time.Local = originalLocal }()

	repoRoot := t.TempDir()
	store := runruntime.New(repoRoot)
	record := agentrun.RunRecord{TicketID: "TKT-410", Role: agentrun.RoleImplementer, RepoRoot: repoRoot, WorktreePath: repoRoot, Branch: "docket/TKT-410", SessionID: "session-410"}
	if err := store.Init(record, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.AppendTranscript("TKT-410", runruntime.TranscriptEntry{At: "2026-03-20T15:48:15Z", Text: "PLAN ticket=TKT-410 steps=2"}); err != nil {
		t.Fatalf("AppendTranscript() error = %v", err)
	}

	var out bytes.Buffer
	repo = repoRoot
	format = "human"
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"tui", "run-log", "TKT-410"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("tui run-log failed: %v\n%s", err, out.String())
	}
	got := out.String()
	if !bytes.Contains([]byte(got), []byte("PLAN ticket=TKT-410 steps=2")) {
		t.Fatalf("unexpected output: %s", got)
	}
	if !bytes.Contains([]byte(got), []byte("Mar 20, 2026 8:48:15 AM PDT")) {
		t.Fatalf("expected localized timestamp in output, got: %s", got)
	}
}

func TestTuiRunLogCmdFailsCleanlyOnCorruptTranscript(t *testing.T) {
	repoRoot := t.TempDir()
	store := runruntime.New(repoRoot)
	record := agentrun.RunRecord{TicketID: "TKT-412", Role: agentrun.RoleImplementer, RepoRoot: repoRoot, WorktreePath: repoRoot, Branch: "docket/TKT-412", SessionID: "session-412"}
	if err := store.Init(record, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	transcriptPath := filepath.Join(store.RunDir("TKT-412"), "transcript.json")
	if err := os.WriteFile(transcriptPath, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write corrupt transcript: %v", err)
	}

	var out bytes.Buffer
	repo = repoRoot
	format = "human"
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"tui", "run-log", "TKT-412"})
	err := rootCmd.Execute()
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("load transcript")) {
		t.Fatalf("expected load transcript error, got err=%v output=%s", err, out.String())
	}
}

func TestRunPingCmdRendersHumanSummary(t *testing.T) {
	prev := newRunOrchestrator
	t.Cleanup(func() {
		newRunOrchestrator = prev
	})
	newRunOrchestrator = func(repoRoot string, enableReview bool) agentrun.Orchestrator {
		return stubRunOrchestrator{
			ping: func(ctx context.Context, ticketID string) (agentrun.PingSummary, error) {
				return agentrun.PingSummary{
					TicketID:  ticketID,
					SessionID: "thread-376",
					Lines: []string{
						"STATUS ticket=TKT-376 phase=testing",
						`SUMMARY ticket=TKT-376 waiting=yes note="still running tests"`,
					},
				}, nil
			},
		}
	}

	var out bytes.Buffer
	repo = t.TempDir()
	format = "human"
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"run-ping", "TKT-376"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("run-ping failed: %v\n%s", err, out.String())
	}
	got := out.String()
	if !bytes.Contains([]byte(got), []byte("TKT-376 session=thread-376")) || !bytes.Contains([]byte(got), []byte("SUMMARY ticket=TKT-376")) {
		t.Fatalf("unexpected output: %s", got)
	}
}

func TestRunWatchLaunchOptionsIncludesCleanupAction(t *testing.T) {
	repoRoot := t.TempDir()
	store := runruntime.New(repoRoot)
	record := agentrun.RunRecord{TicketID: "TKT-411", Role: agentrun.RoleImplementer, RepoRoot: repoRoot, WorktreePath: repoRoot, Branch: "docket/TKT-411", SessionID: "session-411"}
	if err := store.Init(record, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.WriteStatus(runruntime.StatusSnapshot{TicketID: "TKT-411", SessionID: "session-411", Active: false, Hung: true, LastResultStatus: "failed"}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}

	options := runWatchLaunchOptions(repoRoot)
	found := false
	for _, option := range options {
		if option.ID != "clean" {
			continue
		}
		found = true
		if _, err := option.Start(); err != nil {
			t.Fatalf("cleanup action failed: %v", err)
		}
		break
	}
	if !found {
		t.Fatalf("expected cleanup launch option")
	}
	if _, ok, err := store.LoadStatus("TKT-411"); err != nil || ok {
		t.Fatalf("expected cleanup action to remove stale run, ok=%v err=%v", ok, err)
	}
}

func TestRunWatchLaunchOptionsIncludesPingAction(t *testing.T) {
	options := runWatchLaunchOptions(t.TempDir())
	var labels []string
	for _, option := range options {
		labels = append(labels, option.Label)
		if option.ID == "ping" {
			goto found
		}
	}
	t.Fatalf("expected ping launch option")
found:
	if len(options) != 5 {
		t.Fatalf("expected 5 launch options after session-only simplification, got %d (%v)", len(options), labels)
	}
	if labels[0] != "Start Next Ticket" || labels[1] != "Start Auto Cycle" {
		t.Fatalf("unexpected primary launch labels: %v", labels)
	}
}

func TestRunWatchProgramOptionsDisableMouseByDefault(t *testing.T) {
	t.Parallel()

	withoutMouse := runWatchProgramOptions(false)
	withMouse := runWatchProgramOptions(true)

	if len(withoutMouse) != 1 {
		t.Fatalf("expected alt-screen only when mouse is disabled, got %d options", len(withoutMouse))
	}
	if len(withMouse) != 2 {
		t.Fatalf("expected alt-screen plus mouse capture when mouse is enabled, got %d options", len(withMouse))
	}
}

func TestSingleRunSummaryError(t *testing.T) {
	t.Parallel()

	if err := singleRunSummaryError(agentrun.TicketRunSummary{TicketID: "TKT-1", Status: agentrun.StatusDone}); err != nil {
		t.Fatalf("done summary should not error: %v", err)
	}

	err := singleRunSummaryError(agentrun.TicketRunSummary{
		TicketID: "TKT-2",
		Status:   agentrun.StatusFailed,
		Reason:   "pre-commit hook failed",
	})
	if err == nil || err.Error() != "TKT-2 failed: pre-commit hook failed" {
		t.Fatalf("unexpected failed summary error: %v", err)
	}

	if err := singleRunSummaryError(agentrun.TicketRunSummary{
		TicketID: "TKT-3",
		Status:   agentrun.StatusFailed,
		Reason:   "operator requested hard stop",
	}); err != nil {
		t.Fatalf("operator stop should not be treated as launch failure: %v", err)
	}
}

func TestCycleSummaryError(t *testing.T) {
	t.Parallel()

	if err := cycleSummaryError(agentrun.CycleSummary{
		Runs:       []agentrun.TicketRunSummary{{TicketID: "TKT-10", Status: agentrun.StatusDone}},
		StopReason: "operator requested stop after current ticket",
	}); err != nil {
		t.Fatalf("successful stop-after-current should not error: %v", err)
	}

	err := cycleSummaryError(agentrun.CycleSummary{
		Runs:       []agentrun.TicketRunSummary{{TicketID: "TKT-11", Status: agentrun.StatusFailed, Reason: "commit hook failed"}},
		StopReason: "commit hook failed",
	})
	if err == nil || err.Error() != "TKT-11 failed: commit hook failed" {
		t.Fatalf("unexpected cycle failure error: %v", err)
	}

	if err := cycleSummaryError(agentrun.CycleSummary{
		StopReason: "no runnable tickets remain: No workable tickets found. Startable states in current config: backlog, todo. Backlog warning: none are runnable right now; 3 actionable tickets are in startable states, 3 blocked. Top unresolved blockers: TKT-101 x3.",
	}); err != nil {
		t.Fatalf("diagnostic no-runnable stop should not error: %v", err)
	}
}

func TestLaunchManagedSingleRunWithModeReturnsSummaryFailure(t *testing.T) {
	prev := newRunOrchestratorWithMode
	t.Cleanup(func() {
		newRunOrchestratorWithMode = prev
	})

	repoRoot := t.TempDir()
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	store := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	if err := store.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-700",
		Seq:         700,
		Title:       "single failure",
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

	newRunOrchestratorWithMode = func(repoRoot string, enableReview bool, mode string) agentrun.Orchestrator {
		return stubRunOrchestrator{
			runTicket: func(ctx context.Context, ticketID string) (agentrun.TicketRunSummary, error) {
				return agentrun.TicketRunSummary{
					TicketID: ticketID,
					Status:   agentrun.StatusFailed,
					Reason:   "pre-commit hook failed",
				}, nil
			},
		}
	}

	_, err := launchManagedSingleRunWithMode(repoRoot, "session")
	if err == nil || err.Error() != "TKT-700 failed: pre-commit hook failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLaunchManagedSingleRunWithModeReturnsOperatorStopMessage(t *testing.T) {
	prev := newRunOrchestratorWithMode
	t.Cleanup(func() {
		newRunOrchestratorWithMode = prev
	})

	repoRoot := t.TempDir()
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	store := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	if err := store.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-701",
		Seq:         701,
		Title:       "single stop",
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

	newRunOrchestratorWithMode = func(repoRoot string, enableReview bool, mode string) agentrun.Orchestrator {
		return stubRunOrchestrator{
			runTicket: func(ctx context.Context, ticketID string) (agentrun.TicketRunSummary, error) {
				return agentrun.TicketRunSummary{
					TicketID: ticketID,
					Status:   agentrun.StatusFailed,
					Reason:   "operator requested hard stop",
				}, nil
			},
		}
	}

	message, err := launchManagedSingleRunWithMode(repoRoot, "session")
	if err != nil {
		t.Fatalf("operator stop should not error: %v", err)
	}
	if message != "operator requested hard stop" {
		t.Fatalf("unexpected launch message: %q", message)
	}
}

func TestLaunchManagedSingleRunWithModeReturnsDiagnosisWhenNoRunnableTicketExists(t *testing.T) {
	repoRoot := t.TempDir()
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	store := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	for _, tk := range []*ticket.Ticket{
		{
			ID:          "TKT-710",
			Seq:         710,
			Title:       "Current blocker",
			State:       ticket.State("in-progress"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "D",
			AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
		},
		{
			ID:          "TKT-711",
			Seq:         711,
			Title:       "Blocked leaf",
			State:       ticket.State("todo"),
			Priority:    2,
			BlockedBy:   []string{"TKT-710"},
			CreatedAt:   now.Add(time.Minute),
			UpdatedAt:   now.Add(time.Minute),
			CreatedBy:   "human:test",
			Description: "D",
			AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
		},
	} {
		if err := store.CreateTicket(context.Background(), tk); err != nil {
			t.Fatalf("CreateTicket failed: %v", err)
		}
	}

	message, err := launchManagedSingleRunWithMode(repoRoot, "session")
	if err != nil {
		t.Fatalf("expected diagnostic no-runnable launch result, got error: %v", err)
	}
	if !strings.Contains(message, "Backlog warning: none are runnable right now") {
		t.Fatalf("expected diagnosis message, got %q", message)
	}
	if !strings.Contains(message, "Top unresolved blockers: TKT-710 x1") {
		t.Fatalf("expected blocker detail, got %q", message)
	}
}

func TestLaunchManagedAutoCycleWithModeReturnsSummaryFailure(t *testing.T) {
	prev := newRunOrchestratorWithMode
	t.Cleanup(func() {
		newRunOrchestratorWithMode = prev
	})

	repoRoot := t.TempDir()
	newRunOrchestratorWithMode = func(repoRoot string, enableReview bool, mode string) agentrun.Orchestrator {
		return stubRunOrchestrator{
			runNext: func(ctx context.Context) (agentrun.CycleSummary, error) {
				return agentrun.CycleSummary{
					Runs: []agentrun.TicketRunSummary{
						{TicketID: "TKT-701", Status: agentrun.StatusFailed, Reason: "commit hook failed"},
					},
					StopReason: "commit hook failed",
				}, nil
			},
		}
	}

	_, err := launchManagedAutoCycleWithMode(repoRoot, "session")
	if err == nil || err.Error() != "TKT-701 failed: commit hook failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLaunchManagedAutoCycleWithModeAllowsOperatorStopAfterCurrent(t *testing.T) {
	prev := newRunOrchestratorWithMode
	t.Cleanup(func() {
		newRunOrchestratorWithMode = prev
	})

	repoRoot := t.TempDir()
	newRunOrchestratorWithMode = func(repoRoot string, enableReview bool, mode string) agentrun.Orchestrator {
		return stubRunOrchestrator{
			runNext: func(ctx context.Context) (agentrun.CycleSummary, error) {
				return agentrun.CycleSummary{
					Runs: []agentrun.TicketRunSummary{
						{TicketID: "TKT-702", Status: agentrun.StatusDone},
					},
					StopReason: "operator requested stop after current ticket",
				}, nil
			},
		}
	}

	if _, err := launchManagedAutoCycleWithMode(repoRoot, "session"); err != nil {
		t.Fatalf("stop-after-current should not error: %v", err)
	}
}

func TestLaunchManagedAutoCycleWithModePreservesUnderlyingError(t *testing.T) {
	prev := newRunOrchestratorWithMode
	t.Cleanup(func() {
		newRunOrchestratorWithMode = prev
	})

	repoRoot := t.TempDir()
	newRunOrchestratorWithMode = func(repoRoot string, enableReview bool, mode string) agentrun.Orchestrator {
		return stubRunOrchestrator{
			runNext: func(ctx context.Context) (agentrun.CycleSummary, error) {
				return agentrun.CycleSummary{}, errors.New("selector crashed")
			},
		}
	}

	_, err := launchManagedAutoCycleWithMode(repoRoot, "session")
	if err == nil || err.Error() != "selector crashed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorkspaceRunWatchLaunchOptionsIncludePerRepoActions(t *testing.T) {
	workspaceRoot := t.TempDir()
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
	}{
		{repoPath: filepath.Join(workspaceRoot, "goship"), id: "TKT-101"},
		{repoPath: filepath.Join(workspaceRoot, "control-plane"), id: "TKT-201"},
	} {
		if err := os.MkdirAll(filepath.Join(item.repoPath, ".docket"), 0o755); err != nil {
			t.Fatalf("mkdir .docket failed: %v", err)
		}
		if err := ticket.SaveConfig(item.repoPath, ticket.DefaultConfig()); err != nil {
			t.Fatalf("save config failed: %v", err)
		}
		s := local.New(item.repoPath)
		if err := s.CreateTicket(ctx, &ticket.Ticket{
			ID: item.id, Seq: 1, Title: "Runnable", State: ticket.State("todo"), Priority: 1,
			CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{Description: "x"}},
		}); err != nil {
			t.Fatalf("create ticket failed: %v", err)
		}
	}

	repos, err := workspace.Discover(workspaceRoot)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	options, initialRepo, err := workspaceRunWatchLaunchOptions(workspaceRoot, repos)
	if err != nil {
		t.Fatalf("workspaceRunWatchLaunchOptions() error = %v", err)
	}
	if initialRepo == "" {
		t.Fatalf("expected initial repo")
	}
	var labels []string
	for _, option := range options {
		labels = append(labels, option.Label)
	}
	if !containsString(labels, "Start Next Ticket • goship") {
		t.Fatalf("missing goship workspace launch option: %v", labels)
	}
	if !containsString(labels, "Start Next Ticket • control-plane") {
		t.Fatalf("missing control-plane workspace launch option: %v", labels)
	}
}

func TestFormatManagedRunTitle(t *testing.T) {
	if got := formatManagedRunTitle("goship", "TKT-287", "testing", 2, 5, true); got != "goship • TKT-287 • testing • 2/5" {
		t.Fatalf("unexpected title: %q", got)
	}
}

func TestWriteTerminalTitleSkipsNonStdout(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	writeTerminalTitle(cmd, "ignored")
	if out.Len() != 0 {
		t.Fatalf("expected no title write to custom output, got %q", out.String())
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
