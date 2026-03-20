package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/agentrun"
	runruntime "github.com/leomorpho/docket/internal/agentrun/runtime"
)

type stubRunOrchestrator struct {
	runTicket func(ctx context.Context, ticketID string) (agentrun.TicketRunSummary, error)
	runNext   func(ctx context.Context) (agentrun.CycleSummary, error)
	resume    func(ctx context.Context, ticketID string) (agentrun.TicketRunSummary, error)
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
		TicketID:         "TKT-376",
		Active:           true,
		Hung:             true,
		PlannedSteps:     8,
		CurrentStep:      3,
		CurrentStepTitle: "write failing test",
		CurrentPhase:     "testing",
		LastVisibleText:  "STEP ticket=TKT-376 index=3 status=in_progress title=\"write failing test\"",
		LastEventAt:      time.Now().UTC().Format(time.RFC3339Nano),
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
}
