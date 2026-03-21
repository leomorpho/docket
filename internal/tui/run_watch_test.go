package tui

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leomorpho/docket/internal/agentrun"
	runruntime "github.com/leomorpho/docket/internal/agentrun/runtime"
)

func startManagedLikeProcess(t *testing.T) *exec.Cmd {
	t.Helper()
	script := filepath.Join(t.TempDir(), "codex-managed-run.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("write managed-like script: %v", err)
	}
	cmd := exec.Command(script)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start managed-like process: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})
	return cmd
}

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

func TestRunWatchModelStartLaunchOptionSwitchesRepoRoot(t *testing.T) {
	t.Parallel()

	repoA := t.TempDir()
	repoB := t.TempDir()
	model := NewRunWatchModel(repoA, "", nil, false, nil)
	option := RunWatchLaunchOption{
		ID:       "attach:repo-b",
		Label:    "Attach To Active Run • repo-b",
		RepoRoot: repoB,
		Start:    nil,
	}

	updated, _ := model.startLaunchOption(option)
	next := updated.(RunWatchModel)
	if next.repoRoot != repoB {
		t.Fatalf("expected repoRoot to switch to %s, got %s", repoB, next.repoRoot)
	}
	if next.store == nil || !strings.Contains(next.store.RunsRootDir(), repoB) {
		t.Fatalf("expected store repo root to switch to %s", repoB)
	}
}

func TestRunWatchModelMouseWheelScrollsBody(t *testing.T) {
	t.Parallel()

	model := NewRunWatchModel(t.TempDir(), "TKT-500", nil, false, nil)
	model.launchMode = launchModeWatch
	model.followLog = true
	model.scrollOffset = 3

	gotModel, _ := model.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	scrolledUp := gotModel.(RunWatchModel)
	if scrolledUp.scrollOffset != 2 || scrolledUp.followLog {
		t.Fatalf("expected wheel up to scroll body up and disable follow, got %#v", scrolledUp)
	}

	gotModel, _ = scrolledUp.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	scrolledDown := gotModel.(RunWatchModel)
	if scrolledDown.scrollOffset != 3 || scrolledDown.followLog {
		t.Fatalf("expected wheel down to scroll body down and keep follow disabled, got %#v", scrolledDown)
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
	managed := startManagedLikeProcess(t)
	if err := store.WriteStatus(runruntime.StatusSnapshot{TicketID: active.TicketID, SessionID: active.SessionID, PID: managed.Process.Pid, Active: true, LastEventAt: time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
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

func TestLoadRunWatchSnapshotIgnoresStaleNonCodexActiveRun(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := runruntime.New(repoRoot)
	stale := agentrun.RunRecord{TicketID: "TKT-605", Role: agentrun.RoleImplementer, RepoRoot: repoRoot, WorktreePath: repoRoot, Branch: "docket/TKT-605", SessionID: "session-605"}
	active := agentrun.RunRecord{TicketID: "TKT-606", Role: agentrun.RoleImplementer, RepoRoot: repoRoot, WorktreePath: repoRoot, Branch: "docket/TKT-606", SessionID: "session-606"}
	if err := store.Init(stale, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init(stale) error = %v", err)
	}
	if err := store.Init(active, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init(active) error = %v", err)
	}

	sleep := exec.Command("sleep", "5")
	if err := sleep.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	defer func() {
		_ = sleep.Process.Kill()
		_, _ = sleep.Process.Wait()
	}()

	if err := store.WriteStatus(runruntime.StatusSnapshot{
		TicketID:    stale.TicketID,
		SessionID:   stale.SessionID,
		PID:         sleep.Process.Pid,
		Active:      true,
		LastEventAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("WriteStatus(stale) error = %v", err)
	}
	managed := startManagedLikeProcess(t)
	if err := store.WriteStatus(runruntime.StatusSnapshot{
		TicketID:    active.TicketID,
		SessionID:   active.SessionID,
		PID:         managed.Process.Pid,
		Active:      true,
		LastEventAt: time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("WriteStatus(active) error = %v", err)
	}

	snapshot, err := loadRunWatchSnapshot(store, "")
	if err != nil {
		t.Fatalf("loadRunWatchSnapshot() error = %v", err)
	}
	if snapshot.ticketID != active.TicketID || !snapshot.status.Active {
		t.Fatalf("expected live codex-like run to win over stale non-codex active record, got %#v", snapshot)
	}

	staleStatus, ok, err := store.LoadStatus(stale.TicketID)
	if err != nil || !ok {
		t.Fatalf("LoadStatus(stale) ok=%v err=%v", ok, err)
	}
	if staleStatus.Active || !staleStatus.Hung {
		t.Fatalf("expected stale non-codex active record to reconcile to hung, got %#v", staleStatus)
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
		TicketID:            "TKT-700",
		SessionID:           "session-700",
		Active:              true,
		StartedAt:           "2026-03-20T15:40:00Z",
		PlannedSteps:        3,
		CurrentStep:         1,
		CurrentStepTitle:    "inspect repo",
		LastEventAt:         now,
		LastVisibleText:     "PLAN ticket=TKT-700 steps=3",
		SessionMessageCount: 4,
		HealthCheckCount:    2,
		LastHealthCheck:     "SUMMARY ticket=TKT-700 summary=\"still working\"",
		LastIntervention:    "ping",
	}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}
	if err := store.AppendTranscript("TKT-700", runruntime.TranscriptEntry{
		At:   now,
		Text: "PLAN ticket=TKT-700 steps=3",
	}); err != nil {
		t.Fatalf("AppendTranscript() error = %v", err)
	}
	if err := store.AppendStdout("TKT-700", []byte("{\"type\":\"thread.started\",\"thread_id\":\"thread-700\"}\n")); err != nil {
		t.Fatalf("AppendStdout() error = %v", err)
	}
	if err := store.AppendStdout("TKT-700", []byte("{\"type\":\"item.completed\",\"item\":{\"id\":\"item_1\",\"type\":\"assistant_message\",\"content\":[{\"type\":\"output_text\",\"text\":\"I checked the repo\"},{\"type\":\"output_text\",\"text\":\"STATUS ticket=TKT-700 phase=analysis\"}]}}\n")); err != nil {
		t.Fatalf("AppendStdout() error = %v", err)
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
	if !strings.Contains(view, "done 2") || !strings.Contains(view, "Completed This Cycle (2)") || !strings.Contains(view, "TKT-698") || !strings.Contains(view, "[done]") || !strings.Contains(view, "12m") {
		t.Fatalf("view missing cycle completion content: %q", view)
	}
	if !strings.Contains(view, "PROGRESS") || !strings.Contains(view, "33%") {
		t.Fatalf("view missing progress bar content: %q", view)
	}
	if !strings.Contains(view, "still working") || !strings.Contains(view, "ping") {
		t.Fatalf("view missing health/intervention content: %q", view)
	}
	if !strings.Contains(view, "STARTED") || !strings.Contains(view, "LENGTH") {
		t.Fatalf("view missing started/length content: %q", view)
	}
	if !strings.Contains(view, "4") || !strings.Contains(view, "MESSAGES") {
		t.Fatalf("view missing session message count: %q", view)
	}
	if !strings.Contains(view, "Visible Session Log") || !strings.Contains(view, "Codex Session Transcript") {
		t.Fatalf("view missing dual-pane titles: %q", view)
	}
	if !strings.Contains(view, "assistant: I checked the repo") || !strings.Contains(view, "session: thread started thread-700") {
		t.Fatalf("view missing parsed conversation content: %q", view)
	}
}

func TestLoadRunWatchSnapshotPrefersRealCodexSessionTranscript(t *testing.T) {
	repoRoot := t.TempDir()
	store := runruntime.New(repoRoot)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	record := agentrun.RunRecord{
		TicketID:     "TKT-703",
		Role:         agentrun.RoleImplementer,
		RepoRoot:     repoRoot,
		WorktreePath: repoRoot,
		Branch:       "docket/TKT-703",
		SessionID:    "019d0d32-3b5c-7713-a7ce-c739487d21fb",
	}
	if err := store.Init(record, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.WriteStatus(runruntime.StatusSnapshot{
		TicketID:    record.TicketID,
		SessionID:   record.SessionID,
		Active:      true,
		LastEventAt: now,
	}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}
	if err := store.AppendStdout(record.TicketID, []byte("{\"type\":\"thread.started\",\"thread_id\":\"fallback-thread\"}\n")); err != nil {
		t.Fatalf("AppendStdout() error = %v", err)
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	sessionDir := filepath.Join(home, ".codex", "sessions", "2026", "03", "20")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	sessionPath := filepath.Join(sessionDir, "rollout-2026-03-20T14-41-29-019d0d32-3b5c-7713-a7ce-c739487d21fb.jsonl")
	sessionBody := strings.Join([]string{
		`{"timestamp":"2026-03-20T21:41:36.544Z","type":"session_meta","payload":{"id":"019d0d32-3b5c-7713-a7ce-c739487d21fb"}}`,
		`{"timestamp":"2026-03-20T21:41:36.545Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"start work"}]}}`,
		`{"timestamp":"2026-03-20T21:41:36.546Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"PLAN ticket=TKT-703 steps=2"}]}}`,
		`{"timestamp":"2026-03-20T21:41:36.547Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"pwd\"}"}}`,
	}, "\n")
	if err := os.WriteFile(sessionPath, []byte(sessionBody), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	snapshot, err := loadRunWatchSnapshot(store, "TKT-703")
	if err != nil {
		t.Fatalf("loadRunWatchSnapshot() error = %v", err)
	}
	if len(snapshot.conversation) == 0 {
		t.Fatalf("expected conversation lines")
	}
	if !slices.Contains(snapshot.conversation, "session: started 019d0d32-3b5c-7713-a7ce-c739487d21fb") {
		t.Fatalf("expected real codex session transcript, got %#v", snapshot.conversation)
	}
	if !slices.Contains(snapshot.conversation, "user: start work") || !slices.Contains(snapshot.conversation, "assistant: PLAN ticket=TKT-703 steps=2") {
		t.Fatalf("expected message lines from codex session transcript, got %#v", snapshot.conversation)
	}
	if slices.Contains(snapshot.conversation, "session: thread started fallback-thread") {
		t.Fatalf("expected session transcript to take precedence over stdout fallback, got %#v", snapshot.conversation)
	}
}

func TestParseCodexConversationFormatsSessionAndAssistantLines(t *testing.T) {
	t.Parallel()

	got := parseCodexConversation([]string{
		`{"type":"thread.started","thread_id":"thread-123"}`,
		`{"type":"item.completed","item":{"id":"item_1","type":"assistant_message","content":[{"type":"output_text","text":"I checked the repo"},{"type":"output_text","text":"STATUS ticket=TKT-123 phase=analysis"}]}}`,
		`{"type":"item.completed","item":{"id":"item_2","type":"error","message":"Disabled js_repl for this session"}}`,
		`{"type":"response.delta","delta":"thinking"}`,
		`plain raw line`,
	})

	want := []string{
		"session: thread started thread-123",
		"assistant: I checked the repo",
		"assistant: STATUS ticket=TKT-123 phase=analysis",
		"error: Disabled js_repl for this session",
		"event: response.delta",
		"raw: plain raw line",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected parsed conversation:\nwant=%#v\ngot=%#v", want, got)
	}
}

func TestRunWatchViewShowsErrorEventsInConversationPane(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := runruntime.New(repoRoot)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	record := agentrun.RunRecord{
		TicketID:     "TKT-702",
		Role:         agentrun.RoleImplementer,
		RepoRoot:     repoRoot,
		WorktreePath: repoRoot,
		Branch:       "docket/TKT-702",
		SessionID:    "session-702",
	}
	if err := store.Init(record, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.WriteStatus(runruntime.StatusSnapshot{
		TicketID:    "TKT-702",
		SessionID:   "session-702",
		Active:      true,
		LastEventAt: now,
	}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}
	if err := store.AppendStdout("TKT-702", []byte("{\"type\":\"item.completed\",\"item\":{\"id\":\"item_0\",\"type\":\"error\",\"message\":\"Disabled js_repl for this session\"}}\n")); err != nil {
		t.Fatalf("AppendStdout() error = %v", err)
	}

	model := NewRunWatchModel(repoRoot, "TKT-702", nil, false, nil)
	snapshot, err := loadRunWatchSnapshot(store, "TKT-702")
	if err != nil {
		t.Fatalf("loadRunWatchSnapshot() error = %v", err)
	}
	model.snapshot = snapshot
	view := model.View()
	if !strings.Contains(view, "error: Disabled js_repl for this session") {
		t.Fatalf("view missing error event in conversation pane: %q", view)
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

func TestRunWatchModelPingHotkeyInvokesPingAction(t *testing.T) {
	t.Parallel()

	called := false
	model := NewRunWatchModel(t.TempDir(), "", nil, false, []RunWatchLaunchOption{
		{ID: "ping", Label: "Ping Active Session", StayInMenu: true, Start: func() error {
			called = true
			return nil
		}},
	})
	model.launchMode = launchModeWatch

	gotModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if cmd == nil {
		t.Fatalf("expected ping hotkey to return command")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected ping action to be invoked")
	}
	result, ok := msg.(runWatchLaunchResultMsg)
	if !ok {
		t.Fatalf("expected launch result message, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("unexpected ping error: %v", result.err)
	}
	updated, _ := gotModel.Update(msg)
	next := updated.(RunWatchModel)
	if next.launchMode != launchModeMenu {
		t.Fatalf("expected ping action to return to menu, got %s", next.launchMode)
	}
}

func TestRunWatchModelPingHotkeyWithoutActionShowsStatus(t *testing.T) {
	t.Parallel()

	model := NewRunWatchModel(t.TempDir(), "", nil, false, nil)
	model.launchMode = launchModeWatch
	gotModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if cmd != nil {
		t.Fatalf("expected no command when ping action is unavailable")
	}
	updated := gotModel.(RunWatchModel)
	if !strings.Contains(updated.statusMessage, "ping is unavailable") {
		t.Fatalf("unexpected status message: %q", updated.statusMessage)
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
	if !strings.Contains(view, "Visible Session Log") || !strings.Contains(view, "wrapped visible") {
		t.Fatalf("expected visible transcript content in view, got:\n%s", view)
	}
}
