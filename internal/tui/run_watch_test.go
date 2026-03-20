package tui

import (
	"bytes"
	"context"
	"os"
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
	if model.mode != watchModeLog {
		t.Fatalf("expected watch to default to log mode, got %s", model.mode)
	}
	gotModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	toggled := gotModel.(RunWatchModel)
	if toggled.mode != watchModeSummary {
		t.Fatalf("expected toggle to switch to summary mode, got %s", toggled.mode)
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

func TestLoadRunWatchSnapshotIgnoresStaleInactiveRunsWithoutCycle(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := runruntime.New(repoRoot)
	record := agentrun.RunRecord{
		TicketID:     "TKT-602",
		Role:         agentrun.RoleImplementer,
		RepoRoot:     repoRoot,
		WorktreePath: repoRoot,
		Branch:       "docket/TKT-602",
		SessionID:    "session-602",
	}
	if err := store.Init(record, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.WriteStatus(runruntime.StatusSnapshot{
		TicketID:         "TKT-602",
		SessionID:        "session-602",
		Active:           false,
		Hung:             true,
		LastEventAt:      time.Now().UTC().Format(time.RFC3339Nano),
		LastResultStatus: string(agentrun.StatusFailed),
	}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}

	snapshot, err := loadRunWatchSnapshot(store, "")
	if err != nil {
		t.Fatalf("loadRunWatchSnapshot() error = %v", err)
	}
	if snapshot.ticketID != "" || snapshot.statusOK {
		t.Fatalf("expected no active watched ticket, got %#v", snapshot)
	}
}

func TestLoadRunWatchSnapshotPrefersActiveRunWhenCycleIsStale(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := runruntime.New(repoRoot)
	stale := agentrun.RunRecord{TicketID: "TKT-603", Role: agentrun.RoleImplementer, RepoRoot: repoRoot, WorktreePath: repoRoot, Branch: "docket/TKT-603", SessionID: "session-603"}
	active := agentrun.RunRecord{TicketID: "TKT-604", Role: agentrun.RoleImplementer, RepoRoot: repoRoot, WorktreePath: repoRoot, Branch: "docket/TKT-604", SessionID: "session-604"}
	if err := store.Init(stale, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init(stale) error = %v", err)
	}
	if err := store.Init(active, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init(active) error = %v", err)
	}
	if err := store.WriteStatus(runruntime.StatusSnapshot{TicketID: stale.TicketID, SessionID: stale.SessionID, Active: false, Hung: true, LastResultStatus: string(agentrun.StatusFailed)}); err != nil {
		t.Fatalf("WriteStatus(stale) error = %v", err)
	}
	if err := store.WriteStatus(runruntime.StatusSnapshot{TicketID: active.TicketID, SessionID: active.SessionID, PID: os.Getpid(), Active: true, LastEventAt: time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
		t.Fatalf("WriteStatus(active) error = %v", err)
	}
	if err := store.BeginCycle(time.Now()); err != nil {
		t.Fatalf("BeginCycle() error = %v", err)
	}
	if err := store.UpdateCycleCurrent(stale.TicketID, time.Now()); err != nil {
		t.Fatalf("UpdateCycleCurrent() error = %v", err)
	}

	snapshot, err := loadRunWatchSnapshot(store, "")
	if err != nil {
		t.Fatalf("loadRunWatchSnapshot() error = %v", err)
	}
	if snapshot.ticketID != active.TicketID || !snapshot.status.Active {
		t.Fatalf("expected active ticket to win after stale cycle heal, got %#v", snapshot)
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
	if err := store.AppendCycleCompleted("TKT-698", "done", "3m", time.Now()); err != nil {
		t.Fatalf("AppendCycleCompleted() error = %v", err)
	}
	if err := store.AppendCycleCompleted("TKT-699", "done", "12m", time.Now()); err != nil {
		t.Fatalf("AppendCycleCompleted() error = %v", err)
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
	if !strings.Contains(view, "TKT-700") || !strings.Contains(view, "PLAN ticket=TKT-700 steps=3") {
		t.Fatalf("view missing summary content: %q", view)
	}
	if !strings.Contains(view, "Completed This Cycle") || !strings.Contains(view, "TKT-698") || !strings.Contains(view, "12m") {
		t.Fatalf("view missing cycle completion content: %q", view)
	}
	if !strings.Contains(view, "PROGRESS") || !strings.Contains(view, "33%") {
		t.Fatalf("view missing progress bar content: %q", view)
	}
}

func TestRunWatchModelRenderStepBar(t *testing.T) {
	t.Parallel()

	model := NewRunWatchModel(t.TempDir(), "", nil, false, nil)
	model.snapshot.statusOK = true
	model.snapshot.status = runruntime.StatusSnapshot{
		PlannedSteps: 4,
		CurrentStep:  2,
	}
	got := model.renderStepBar()
	if !strings.Contains(got, "50%") {
		t.Fatalf("expected 50%% progress, got %q", got)
	}

	model.snapshot.status = runruntime.StatusSnapshot{}
	got = model.renderStepBar()
	if !strings.Contains(got, "waiting") {
		t.Fatalf("expected waiting fallback, got %q", got)
	}
}

func TestRunWatchModelViewFormatsLastEventInLocalTime(t *testing.T) {
	t.Parallel()

	originalLocal := time.Local
	time.Local = time.FixedZone("PDT", -7*60*60)
	defer func() { time.Local = originalLocal }()

	repoRoot := t.TempDir()
	store := runruntime.New(repoRoot)
	now := "2026-03-20T15:48:15.187027Z"
	record := agentrun.RunRecord{
		TicketID:     "TKT-701",
		Role:         agentrun.RoleImplementer,
		RepoRoot:     repoRoot,
		WorktreePath: repoRoot,
		Branch:       "docket/TKT-701",
		SessionID:    "session-701",
	}
	if err := store.Init(record, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.WriteStatus(runruntime.StatusSnapshot{
		TicketID:    "TKT-701",
		SessionID:   "session-701",
		Active:      true,
		LastEventAt: now,
	}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}

	model := NewRunWatchModel(repoRoot, "TKT-701", nil, false, nil)
	snapshot, err := loadRunWatchSnapshot(store, "TKT-701")
	if err != nil {
		t.Fatalf("loadRunWatchSnapshot() error = %v", err)
	}
	model.snapshot = snapshot
	view := model.View()
	if strings.Contains(view, now) {
		t.Fatalf("view should not render raw RFC3339 timestamp: %q", view)
	}
	if !strings.Contains(view, "Mar 20, 2026 8:48:15 AM PDT") {
		t.Fatalf("view missing localized timestamp: %q", view)
	}
}

func TestRunWatchModelViewKeepsRunFinishedInFooterNotBanner(t *testing.T) {
	t.Parallel()

	model := NewRunWatchModel(t.TempDir(), "", nil, false, nil)
	model.showDoneNotice = true
	view := model.View()
	if strings.Contains(view, "Run finished\n") && strings.Contains(view, "╭") {
		// ensure we do not render a separate status card just for finished state
		if strings.Contains(view, "Run finished\n  │") || strings.Contains(view, "│  Run finished") {
			t.Fatalf("view rendered separate finished banner: %q", view)
		}
	}
	if !strings.Contains(strings.ToLower(view), "run finished") {
		t.Fatalf("view missing footer finished status: %q", view)
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
	if !strings.Contains(view, "Select mode") || !strings.Contains(view, "Attach To Active Run") {
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

func TestRunWatchModelFollowAndOverviewControls(t *testing.T) {
	t.Parallel()

	model := NewRunWatchModel(t.TempDir(), "", nil, false, nil)
	model.snapshot.transcript = []runruntime.TranscriptEntry{
		{Text: "line 1"},
		{Text: "line 2"},
		{Text: "line 3"},
	}
	model.height = 10

	gotModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	toggledOverview := gotModel.(RunWatchModel)
	if toggledOverview.showOverview {
		t.Fatalf("expected h to hide overview")
	}

	gotModel, _ = toggledOverview.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	scrolled := gotModel.(RunWatchModel)
	if scrolled.followLog {
		t.Fatalf("expected manual scroll to disable follow mode")
	}

	gotModel, _ = scrolled.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	following := gotModel.(RunWatchModel)
	if !following.followLog {
		t.Fatalf("expected G to re-enable follow mode")
	}

	gotModel, _ = following.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	top := gotModel.(RunWatchModel)
	if top.followLog || top.scrollOffset != 0 {
		t.Fatalf("expected g to jump to top and disable follow, got %#v", top)
	}

	gotModel, _ = top.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	help := gotModel.(RunWatchModel)
	if !help.showHelp {
		t.Fatalf("expected ? to toggle help")
	}
}

func TestRunWatchModelHardStopRequiresConfirmation(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := runruntime.New(repoRoot)
	record := agentrun.RunRecord{TicketID: "TKT-889", Role: agentrun.RoleImplementer, RepoRoot: repoRoot, WorktreePath: repoRoot, Branch: "docket/TKT-889", SessionID: "session-889"}
	if err := store.Init(record, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.WriteStatus(runruntime.StatusSnapshot{TicketID: record.TicketID, SessionID: record.SessionID, PID: 999999, Active: true}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}
	model := NewRunWatchModel(repoRoot, "TKT-889", nil, false, nil)
	model.store = store
	model.snapshot.ticketID = "TKT-889"
	model.snapshot.status = runruntime.StatusSnapshot{TicketID: "TKT-889", SessionID: "session-889", PID: 999999, Active: true}
	model.snapshot.statusOK = true

	gotModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	confirm := gotModel.(RunWatchModel)
	if !confirm.confirmHardStop {
		t.Fatalf("expected first x to arm confirmation")
	}
	if !strings.Contains(confirm.View(), "hard stop armed") {
		t.Fatalf("expected view to show armed hard-stop state")
	}
	gotModel, _ = confirm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	stopped := gotModel.(RunWatchModel)
	if stopped.confirmHardStop {
		t.Fatalf("expected confirmation to clear after hard stop")
	}
	status, ok, err := store.LoadStatus("TKT-889")
	if err != nil || !ok {
		t.Fatalf("LoadStatus() ok=%v err=%v", ok, err)
	}
	if status.Active || status.LastResultStatus != "stopped" {
		t.Fatalf("expected stopped runtime status, got %#v", status)
	}
}

func TestRunWatchModelMenuCleanupActionStaysInMenu(t *testing.T) {
	t.Parallel()

	model := NewRunWatchModel(t.TempDir(), "", nil, false, []RunWatchLaunchOption{
		{ID: "clean", Label: "Clean Stale Runs", StayInMenu: true, Start: func() error { return nil }},
	})
	gotModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected cleanup action to return command")
	}
	msg := cmd()
	updated, _ := gotModel.Update(msg)
	back := updated.(RunWatchModel)
	if back.launchMode != launchModeMenu {
		t.Fatalf("expected cleanup action to remain in menu, got %s", back.launchMode)
	}
}

func TestRunWatchProgramQuitsOnQ(t *testing.T) {
	t.Parallel()

	model := NewRunWatchModel(t.TempDir(), "", nil, false, []RunWatchLaunchOption{
		{ID: "attach", Label: "Attach To Active Run"},
	})
	var input bytes.Buffer
	input.WriteString("q")
	var output bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	program := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithInput(&input),
		tea.WithOutput(&output),
		tea.WithoutRenderer(),
	)
	if _, err := program.Run(); err != nil {
		t.Fatalf("program.Run() error = %v", err)
	}
}

func TestRunWatchModelViewWrapsLongVisibleLines(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := runruntime.New(repoRoot)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	record := agentrun.RunRecord{
		TicketID:     "TKT-888",
		Role:         agentrun.RoleImplementer,
		RepoRoot:     repoRoot,
		WorktreePath: repoRoot,
		Branch:       "docket/TKT-888",
		SessionID:    "session-888",
	}
	if err := store.Init(record, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.WriteStatus(runruntime.StatusSnapshot{
		TicketID:        "TKT-888",
		SessionID:       "session-888",
		Active:          true,
		LastEventAt:     now,
		LastVisibleText: strings.Repeat("very long visible text ", 8),
	}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}
	if err := store.AppendTranscript("TKT-888", runruntime.TranscriptEntry{
		At:   now,
		Text: strings.Repeat("wrapped visible transcript line ", 8),
	}); err != nil {
		t.Fatalf("AppendTranscript() error = %v", err)
	}

	model := NewRunWatchModel(repoRoot, "TKT-888", nil, false, nil)
	model.width = 60
	model.height = 20
	snapshot, err := loadRunWatchSnapshot(store, "TKT-888")
	if err != nil {
		t.Fatalf("loadRunWatchSnapshot() error = %v", err)
	}
	model.snapshot = snapshot
	view := model.View()
	if strings.Contains(view, strings.Repeat("wrapped visible transcript line ", 4)) {
		t.Fatalf("expected long transcript text to wrap, got:\n%s", view)
	}
	if !strings.Contains(view, "wrapped visible transcript line") {
		t.Fatalf("expected visible transcript content in view, got:\n%s", view)
	}
}
