package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leomorpho/docket/internal/agentrun"
	runruntime "github.com/leomorpho/docket/internal/agentrun/runtime"
)

func TestRunWatchModelToggleAndStopRequest(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := runruntime.New(repoRoot)
	record := agentrun.RunRecord{
		TicketID:     "TKT-500",
		Role:         agentrun.RoleImplementer,
		RepoRoot:     repoRoot,
		WorktreePath: repoRoot,
		Branch:       "docket/TKT-500",
		SessionID:    "session-500",
	}
	if err := store.Init(record, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.BeginCycle(time.Now()); err != nil {
		t.Fatalf("BeginCycle() error = %v", err)
	}
	if err := store.UpdateCycleCurrent("TKT-500", time.Now()); err != nil {
		t.Fatalf("UpdateCycleCurrent() error = %v", err)
	}

	model := NewRunWatchModel(repoRoot, "TKT-500", nil, false, nil)
	gotModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	toggled := gotModel.(RunWatchModel)
	if toggled.mode != watchModeLog {
		t.Fatalf("expected log mode, got %s", toggled.mode)
	}
	gotModel, _ = toggled.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	afterStop := gotModel.(RunWatchModel)
	state, ok, err := store.LoadCycleState()
	if err != nil || !ok {
		t.Fatalf("LoadCycleState() ok=%v err=%v", ok, err)
	}
	if !state.StopAfterCurrent || !afterStop.snapshot.cycle.StopAfterCurrent {
		t.Fatalf("expected stop-after-current request, state=%#v model=%#v", state, afterStop.snapshot)
	}
}

func TestLoadRunWatchSnapshotPrefersCycleCurrentTicket(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := runruntime.New(repoRoot)
	now := time.Now()
	for _, record := range []agentrun.RunRecord{
		{TicketID: "TKT-600", Role: agentrun.RoleImplementer, RepoRoot: repoRoot, WorktreePath: repoRoot, Branch: "docket/TKT-600", SessionID: "session-600"},
		{TicketID: "TKT-601", Role: agentrun.RoleImplementer, RepoRoot: repoRoot, WorktreePath: repoRoot, Branch: "docket/TKT-601", SessionID: "session-601"},
	} {
		if err := store.Init(record, "prompt", 10*time.Minute); err != nil {
			t.Fatalf("Init(%s) error = %v", record.TicketID, err)
		}
		if err := store.WriteStatus(runruntime.StatusSnapshot{
			TicketID:    record.TicketID,
			SessionID:   record.SessionID,
			Active:      true,
			LastEventAt: now.UTC().Format(time.RFC3339Nano),
		}); err != nil {
			t.Fatalf("WriteStatus(%s) error = %v", record.TicketID, err)
		}
	}
	if err := store.BeginCycle(now); err != nil {
		t.Fatalf("BeginCycle() error = %v", err)
	}
	if err := store.UpdateCycleCurrent("TKT-601", now); err != nil {
		t.Fatalf("UpdateCycleCurrent() error = %v", err)
	}
	if err := store.AppendTranscript("TKT-601", runruntime.TranscriptEntry{At: now.UTC().Format(time.RFC3339Nano), Text: "PLAN ticket=TKT-601 steps=2"}); err != nil {
		t.Fatalf("AppendTranscript() error = %v", err)
	}

	snapshot, err := loadRunWatchSnapshot(store, "")
	if err != nil {
		t.Fatalf("loadRunWatchSnapshot() error = %v", err)
	}
	if snapshot.ticketID != "TKT-601" || len(snapshot.transcript) != 1 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
}

func TestRunWatchModelViewShowsKeyLegendAndSummary(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := runruntime.New(repoRoot)
	now := time.Now().UTC().Format(time.RFC3339Nano)
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
	if err := store.BeginCycle(time.Now()); err != nil {
		t.Fatalf("BeginCycle() error = %v", err)
	}
	if err := store.UpdateCycleCurrent("TKT-700", time.Now()); err != nil {
		t.Fatalf("UpdateCycleCurrent() error = %v", err)
	}
	if err := store.WriteStatus(runruntime.StatusSnapshot{
		TicketID:         "TKT-700",
		SessionID:        "session-700",
		Active:           true,
		PlannedSteps:     3,
		CurrentStep:      1,
		CurrentStepTitle: "inspect repo",
		LastEventAt:      now,
		LastVisibleText:  "PLAN ticket=TKT-700 steps=3",
	}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}
	if err := store.AppendTranscript("TKT-700", runruntime.TranscriptEntry{
		At:   now,
		Text: "PLAN ticket=TKT-700 steps=3",
	}); err != nil {
		t.Fatalf("AppendTranscript() error = %v", err)
	}

	model := NewRunWatchModel(repoRoot, "TKT-700", nil, false, nil)
	snapshot, err := loadRunWatchSnapshot(store, "TKT-700")
	if err != nil {
		t.Fatalf("loadRunWatchSnapshot() error = %v", err)
	}
	model.snapshot = snapshot
	view := model.View()
	if !strings.Contains(view, "keys: "+model.runWatchKeyLegend()) {
		t.Fatalf("view missing key legend: %q", view)
	}
	if strings.Contains(view, "m menu") {
		t.Fatalf("view should not advertise menu key without launcher options: %q", view)
	}
	if !strings.Contains(view, "ticket: TKT-700") || !strings.Contains(view, "inspect repo") {
		t.Fatalf("view missing summary content: %q", view)
	}
}

func TestRunWatchModelViewShowsMenuKeyWhenLauncherEnabled(t *testing.T) {
	t.Parallel()

	model := NewRunWatchModel(t.TempDir(), "", nil, false, []RunWatchLaunchOption{
		{ID: "single", Label: "Start Next Ticket"},
	})
	gotModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view := gotModel.(RunWatchModel).View()
	if !strings.Contains(view, "m menu") {
		t.Fatalf("view missing menu key legend: %q", view)
	}
}

func TestRunWatchModelMenuLaunchesAttachMode(t *testing.T) {
	t.Parallel()

	model := NewRunWatchModel(t.TempDir(), "", nil, false, []RunWatchLaunchOption{
		{ID: "attach", Label: "Attach To Active Run"},
	})
	view := model.View()
	if !strings.Contains(view, "Select mode:") || !strings.Contains(view, "Attach To Active Run") {
		t.Fatalf("menu view missing launcher content: %q", view)
	}

	gotModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("attach option should not return an async command")
	}
	updated := gotModel.(RunWatchModel)
	if updated.launchMode != launchModeWatch {
		t.Fatalf("expected attach option to switch to watch mode, got %s", updated.launchMode)
	}
}

func TestRunWatchModelMenuLaunchInvokesStartCallback(t *testing.T) {
	t.Parallel()

	called := false
	model := NewRunWatchModel(t.TempDir(), "", nil, false, []RunWatchLaunchOption{
		{
			ID:    "single",
			Label: "Start Next Ticket",
			Start: func() error {
				called = true
				return nil
			},
		},
	})

	gotModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("launch option should return an async command")
	}
	launching := gotModel.(RunWatchModel)
	if !launching.launching || launching.launchMode != launchModeWatch {
		t.Fatalf("expected launching watch mode, got %#v", launching)
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected launch callback to be invoked")
	}
	result, ok := msg.(runWatchLaunchResultMsg)
	if !ok {
		t.Fatalf("expected launch result message, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("unexpected launch error: %v", result.err)
	}
}

func TestRunWatchModelMenuNavigationAndReturn(t *testing.T) {
	t.Parallel()

	model := NewRunWatchModel(t.TempDir(), "", nil, false, []RunWatchLaunchOption{
		{ID: "single", Label: "Start Next Ticket"},
		{ID: "auto", Label: "Start Auto Cycle"},
	})
	gotModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	updated := gotModel.(RunWatchModel)
	if updated.selectedOption != 1 {
		t.Fatalf("expected second option selected, got %d", updated.selectedOption)
	}
	gotModel, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	watching := gotModel.(RunWatchModel)
	if watching.launchMode != launchModeWatch {
		t.Fatalf("expected watch mode after enter, got %s", watching.launchMode)
	}
	gotModel, _ = watching.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	backToMenu := gotModel.(RunWatchModel)
	if backToMenu.launchMode != launchModeMenu {
		t.Fatalf("expected menu mode after m, got %s", backToMenu.launchMode)
	}
}
