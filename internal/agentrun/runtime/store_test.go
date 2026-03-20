package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/agentrun"
)

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
	state, ok, err := store.LoadCycleState()
	if err != nil || !ok {
		t.Fatalf("LoadCycleState() ok=%v err=%v", ok, err)
	}
	if !state.Active || state.CurrentTicketID != "TKT-400" || !state.StopAfterCurrent {
		t.Fatalf("unexpected cycle state: %#v", state)
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
