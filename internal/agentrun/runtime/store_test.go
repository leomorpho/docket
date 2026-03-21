package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/agentrun"
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

func TestStoreInitAppendSnapshotAndCleanup(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := New(repoRoot)
	record := agentrun.RunRecord{
		TicketID:     "TKT-390",
		Role:         agentrun.RoleImplementer,
		Adapter:      "codex",
		RepoRoot:     repoRoot,
		WorktreePath: filepath.Join(repoRoot, "wt"),
		Branch:       "docket/TKT-390",
		StartedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:    "session-390",
	}

	if err := store.Init(record, "prompt body", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.AppendStdout(record.TicketID, []byte("{\"type\":\"turn.started\"}\n")); err != nil {
		t.Fatalf("AppendStdout() error = %v", err)
	}
	if err := store.AppendStderr(record.TicketID, []byte("trace\n")); err != nil {
		t.Fatalf("AppendStderr() error = %v", err)
	}
	if err := store.AppendTranscript(record.TicketID, TranscriptEntry{
		At:   time.Now().UTC().Format(time.RFC3339Nano),
		Text: "PLAN ticket=TKT-390 steps=4",
	}); err != nil {
		t.Fatalf("AppendTranscript() error = %v", err)
	}
	if err := store.WriteStatus(StatusSnapshot{
		TicketID:          record.TicketID,
		SessionID:         record.SessionID,
		Active:            true,
		LastMarker:        "PLAN",
		InactivityTimeout: "10m0s",
	}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}

	status, ok, err := store.LoadStatus(record.TicketID)
	if err != nil || !ok {
		t.Fatalf("LoadStatus() ok=%v err=%v", ok, err)
	}
	if status.LastMarker != "PLAN" {
		t.Fatalf("unexpected status: %#v", status)
	}
	if status.StartedAt == "" {
		t.Fatalf("expected started_at to be persisted: %#v", status)
	}
	transcript, err := store.LoadTranscript(record.TicketID)
	if err != nil {
		t.Fatalf("LoadTranscript() error = %v", err)
	}
	if len(transcript) != 1 || transcript[0].Text != "PLAN ticket=TKT-390 steps=4" {
		t.Fatalf("unexpected transcript: %#v", transcript)
	}
	prompt, err := store.LoadPrompt(record.TicketID)
	if err != nil {
		t.Fatalf("LoadPrompt() error = %v", err)
	}
	if prompt != "prompt body" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}

	raw, err := os.ReadFile(filepath.Join(store.RunDir(record.TicketID), stdoutFile))
	if err != nil {
		t.Fatalf("read stdout file: %v", err)
	}
	if !strings.Contains(string(raw), "turn.started") {
		t.Fatalf("stdout missing data: %s", string(raw))
	}
	stdoutLines, err := store.LoadStdoutLines(record.TicketID)
	if err != nil {
		t.Fatalf("LoadStdoutLines() error = %v", err)
	}
	if len(stdoutLines) != 1 || !strings.Contains(stdoutLines[0], "turn.started") {
		t.Fatalf("unexpected stdout lines: %#v", stdoutLines)
	}

	if err := store.Cleanup(record.TicketID); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if _, ok, err := store.LoadStatus(record.TicketID); err != nil || ok {
		t.Fatalf("expected cleaned status, ok=%v err=%v", ok, err)
	}
}

func TestStoreInitResetsPreviousActiveRunArtifacts(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := New(repoRoot)
	record := agentrun.RunRecord{
		TicketID:     "TKT-391",
		Role:         agentrun.RoleImplementer,
		Adapter:      "codex",
		RepoRoot:     repoRoot,
		WorktreePath: filepath.Join(repoRoot, "wt"),
		Branch:       "docket/TKT-391",
		StartedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:    "session-old",
	}

	if err := store.Init(record, "first prompt", 10*time.Minute); err != nil {
		t.Fatalf("first Init() error = %v", err)
	}
	if err := store.AppendStdout(record.TicketID, []byte("old-line\n")); err != nil {
		t.Fatalf("AppendStdout() error = %v", err)
	}
	if err := store.AppendTranscript(record.TicketID, TranscriptEntry{At: time.Now().UTC().Format(time.RFC3339Nano), Text: "PLAN ticket=TKT-391 steps=9"}); err != nil {
		t.Fatalf("AppendTranscript() error = %v", err)
	}

	record.SessionID = "session-new"
	if err := store.Init(record, "second prompt", 5*time.Minute); err != nil {
		t.Fatalf("second Init() error = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(store.RunDir(record.TicketID), stdoutFile))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read stdout file: %v", err)
	}
	if strings.Contains(string(raw), "old-line") {
		t.Fatalf("expected Init() to clear prior stdout, got %q", string(raw))
	}
	transcript, err := store.LoadTranscript(record.TicketID)
	if err != nil {
		t.Fatalf("LoadTranscript() error = %v", err)
	}
	if len(transcript) != 0 {
		t.Fatalf("expected transcript reset, got %#v", transcript)
	}
	prompt, err := store.LoadPrompt(record.TicketID)
	if err != nil {
		t.Fatalf("LoadPrompt() error = %v", err)
	}
	if prompt != "second prompt" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
	status, ok, err := store.LoadStatus(record.TicketID)
	if err != nil || !ok {
		t.Fatalf("LoadStatus() ok=%v err=%v", ok, err)
	}
	if status.SessionID != "session-new" || status.InactivityTimeout != "5m0s" {
		t.Fatalf("unexpected reset status: %#v", status)
	}
}

func TestStoreCycleStateAndStopRequestLifecycle(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	now := time.Date(2026, 3, 20, 5, 0, 0, 0, time.UTC)
	if err := store.BeginCycle(now); err != nil {
		t.Fatalf("BeginCycle() error = %v", err)
	}
	if err := store.UpdateCycleCurrent("TKT-400", now.Add(time.Second)); err != nil {
		t.Fatalf("UpdateCycleCurrent() error = %v", err)
	}
	if err := store.RequestStopAfterCurrent(now.Add(2 * time.Second)); err != nil {
		t.Fatalf("RequestStopAfterCurrent() error = %v", err)
	}
	if err := store.AppendCycleCompleted("TKT-399", "done", "4m", now.Add(3*time.Second)); err != nil {
		t.Fatalf("AppendCycleCompleted() error = %v", err)
	}
	state, ok, err := store.LoadCycleState()
	if err != nil || !ok {
		t.Fatalf("LoadCycleState() ok=%v err=%v", ok, err)
	}
	if !state.Active || state.CurrentTicketID != "TKT-400" || !state.StopAfterCurrent {
		t.Fatalf("unexpected cycle state: %#v", state)
	}
	if len(state.Completed) != 1 || state.Completed[0].TicketID != "TKT-399" || state.Completed[0].Length != "4m" {
		t.Fatalf("unexpected completed cycle runs: %#v", state.Completed)
	}
	stopRequested, err := store.StopAfterCurrentRequested()
	if err != nil {
		t.Fatalf("StopAfterCurrentRequested() error = %v", err)
	}
	if !stopRequested {
		t.Fatalf("expected stop request to be true")
	}
	if err := store.EndCycle(); err != nil && !os.IsNotExist(err) {
		t.Fatalf("EndCycle() error = %v", err)
	}
	if _, ok, err := store.LoadCycleState(); err != nil || ok {
		t.Fatalf("expected no cycle state after EndCycle, ok=%v err=%v", ok, err)
	}
}

func TestLoadStatusReconcilesDeadActiveProcessToHung(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := New(repoRoot)
	record := agentrun.RunRecord{
		TicketID:     "TKT-401",
		Role:         agentrun.RoleImplementer,
		Adapter:      "codex",
		RepoRoot:     repoRoot,
		WorktreePath: filepath.Join(repoRoot, "wt"),
		Branch:       "docket/TKT-401",
		StartedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:    "session-401",
	}
	if err := store.Init(record, "prompt body", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.WriteStatus(StatusSnapshot{
		TicketID:          record.TicketID,
		SessionID:         record.SessionID,
		PID:               999999,
		Active:            true,
		InactivityTimeout: "10m0s",
	}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}

	status, ok, err := store.LoadStatus(record.TicketID)
	if err != nil || !ok {
		t.Fatalf("LoadStatus() ok=%v err=%v", ok, err)
	}
	if status.Active || !status.Hung || status.LastResultStatus != string(agentrun.StatusFailed) {
		t.Fatalf("expected dead process to reconcile to hung failed state, got %#v", status)
	}
}

func TestLoadStatusReconcilesNonCodexReusedPIDToHung(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := New(repoRoot)
	record := agentrun.RunRecord{
		TicketID:     "TKT-405",
		Role:         agentrun.RoleImplementer,
		Adapter:      "codex",
		RepoRoot:     repoRoot,
		WorktreePath: filepath.Join(repoRoot, "wt"),
		Branch:       "docket/TKT-405",
		StartedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:    "session-405",
	}
	if err := store.Init(record, "prompt body", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	sleep := exec.Command("sleep", "5")
	if err := sleep.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	defer func() {
		_ = sleep.Process.Kill()
		_, _ = sleep.Process.Wait()
	}()

	if err := store.WriteStatus(StatusSnapshot{
		TicketID:  record.TicketID,
		SessionID: record.SessionID,
		PID:       sleep.Process.Pid,
		Active:    true,
	}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}

	status, ok, err := store.LoadStatus(record.TicketID)
	if err != nil || !ok {
		t.Fatalf("LoadStatus() ok=%v err=%v", ok, err)
	}
	if status.Active || !status.Hung {
		t.Fatalf("expected non-codex reused pid to reconcile to hung status, got %#v", status)
	}
}

func TestStoreHealRuntimeStateClearsStaleCycle(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := New(repoRoot)
	record := agentrun.RunRecord{
		TicketID:     "TKT-402",
		Role:         agentrun.RoleImplementer,
		RepoRoot:     repoRoot,
		WorktreePath: filepath.Join(repoRoot, "wt"),
		Branch:       "docket/TKT-402",
		SessionID:    "session-402",
	}
	if err := store.Init(record, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.WriteStatus(StatusSnapshot{
		TicketID:         record.TicketID,
		SessionID:        record.SessionID,
		Active:           false,
		Hung:             true,
		LastResultStatus: string(agentrun.StatusFailed),
	}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}
	if err := store.BeginCycle(time.Now()); err != nil {
		t.Fatalf("BeginCycle() error = %v", err)
	}
	if err := store.UpdateCycleCurrent(record.TicketID, time.Now()); err != nil {
		t.Fatalf("UpdateCycleCurrent() error = %v", err)
	}

	warnings, err := store.HealRuntimeState(time.Now())
	if err != nil {
		t.Fatalf("HealRuntimeState() error = %v", err)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected stale cycle warning")
	}
	if _, ok, err := store.LoadCycleState(); err != nil || ok {
		t.Fatalf("expected cycle to be cleared, ok=%v err=%v", ok, err)
	}
}

func TestStoreCleanupStaleRunsRemovesOnlyInactiveRuns(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := New(repoRoot)
	active := agentrun.RunRecord{TicketID: "TKT-403", Role: agentrun.RoleImplementer, RepoRoot: repoRoot, WorktreePath: repoRoot, Branch: "docket/TKT-403", SessionID: "session-403"}
	stale := agentrun.RunRecord{TicketID: "TKT-404", Role: agentrun.RoleImplementer, RepoRoot: repoRoot, WorktreePath: repoRoot, Branch: "docket/TKT-404", SessionID: "session-404"}
	if err := store.Init(active, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init(active) error = %v", err)
	}
	if err := store.Init(stale, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init(stale) error = %v", err)
	}
	managed := startManagedLikeProcess(t)
	if err := store.WriteStatus(StatusSnapshot{TicketID: active.TicketID, SessionID: active.SessionID, PID: managed.Process.Pid, Active: true}); err != nil {
		t.Fatalf("WriteStatus(active) error = %v", err)
	}
	if err := store.WriteStatus(StatusSnapshot{TicketID: stale.TicketID, SessionID: stale.SessionID, Active: false, Hung: true, LastResultStatus: "stopped"}); err != nil {
		t.Fatalf("WriteStatus(stale) error = %v", err)
	}

	removed, err := store.CleanupStaleRuns()
	if err != nil {
		t.Fatalf("CleanupStaleRuns() error = %v", err)
	}
	if len(removed) != 1 || removed[0] != stale.TicketID {
		t.Fatalf("unexpected removed set: %#v", removed)
	}
	if _, ok, err := store.LoadStatus(active.TicketID); err != nil || !ok {
		t.Fatalf("expected active run to remain, ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.LoadStatus(stale.TicketID); err != nil || ok {
		t.Fatalf("expected stale run to be removed, ok=%v err=%v", ok, err)
	}
}

func TestStoreHardStopRunMarksStoppedAndRequestsCycleStop(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := New(repoRoot)
	record := agentrun.RunRecord{TicketID: "TKT-405", Role: agentrun.RoleImplementer, RepoRoot: repoRoot, WorktreePath: repoRoot, Branch: "docket/TKT-405", SessionID: "session-405"}
	if err := store.Init(record, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.WriteStatus(StatusSnapshot{TicketID: record.TicketID, SessionID: record.SessionID, PID: 999999, Active: true}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}
	if err := store.BeginCycle(time.Now()); err != nil {
		t.Fatalf("BeginCycle() error = %v", err)
	}
	if err := store.UpdateCycleCurrent(record.TicketID, time.Now()); err != nil {
		t.Fatalf("UpdateCycleCurrent() error = %v", err)
	}

	if err := store.HardStopRun(record.TicketID, time.Now()); err != nil {
		t.Fatalf("HardStopRun() error = %v", err)
	}
	status, ok, err := store.LoadStatus(record.TicketID)
	if err != nil || !ok {
		t.Fatalf("LoadStatus() ok=%v err=%v", ok, err)
	}
	if status.Active || status.Hung || status.LastResultStatus != "stopped" {
		t.Fatalf("unexpected status after hard stop: %#v", status)
	}
	stopRequested, err := store.StopAfterCurrentRequested()
	if err != nil {
		t.Fatalf("StopAfterCurrentRequested() error = %v", err)
	}
	if !stopRequested {
		t.Fatalf("expected cycle stop to be requested")
	}
}
