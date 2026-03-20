package monitor

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/agentrun"
	runruntime "github.com/leomorpho/docket/internal/agentrun/runtime"
)

type fakeHandle struct {
	stdout io.Reader
	stderr io.Reader
	waitCh chan error
	killed bool
}

func (h *fakeHandle) Stdout() io.Reader { return h.stdout }
func (h *fakeHandle) Stderr() io.Reader { return h.stderr }
func (h *fakeHandle) Wait() error       { return <-h.waitCh }
func (h *fakeHandle) Kill() error {
	h.killed = true
	select {
	case h.waitCh <- errors.New("killed"):
	default:
	}
	return nil
}
func (h *fakeHandle) PID() int { return 99 }

func TestObserverParsesStructuredResultLine(t *testing.T) {
	t.Parallel()

	handle := &fakeHandle{
		stdout: bytes.NewBufferString("noise\nRESULT status=done ticket=TKT-381 role=implementer commit=abc123 tests=passed\n"),
		stderr: bytes.NewReader(nil),
		waitCh: make(chan error, 1),
	}
	handle.waitCh <- nil

	obs, err := New().Observe(context.Background(), agentrun.ObservationInput{
		Handle:  handle,
		Record:  agentrun.RunRecord{TicketID: "TKT-381", Role: agentrun.RoleImplementer},
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if obs.Result.Status != agentrun.StatusDone {
		t.Fatalf("unexpected observation: %#v", obs)
	}
	if obs.Result.CommitSHA != "abc123" {
		t.Fatalf("unexpected observation: %#v", obs)
	}
}

func TestObserverParsesStructuredResultFromCodexJSONEvent(t *testing.T) {
	t.Parallel()

	handle := &fakeHandle{
		stdout: bytes.NewBufferString("{\"type\":\"item.completed\",\"item\":{\"id\":\"item_1\",\"type\":\"agent_message\",\"text\":\"RESULT status=done ticket=TKT-381 role=implementer commit=abc123 tests=passed\"}}\n"),
		stderr: bytes.NewBufferString("2026-03-20T03:03:28Z ERROR ignored preface\n"),
		waitCh: make(chan error, 1),
	}
	handle.waitCh <- nil

	obs, err := New().Observe(context.Background(), agentrun.ObservationInput{
		Handle:  handle,
		Record:  agentrun.RunRecord{TicketID: "TKT-381", Role: agentrun.RoleImplementer},
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if obs.Result.Status != agentrun.StatusDone || obs.Result.CommitSHA != "abc123" {
		t.Fatalf("unexpected observation: %#v", obs)
	}
}

func TestObserverCapturesPlainStdoutVisibleText(t *testing.T) {
	t.Parallel()

	handle := &fakeHandle{
		stdout: bytes.NewBufferString("I am reading the code now\nRESULT status=done ticket=TKT-381 role=implementer commit=abc123 tests=passed\n"),
		stderr: bytes.NewReader(nil),
		waitCh: make(chan error, 1),
	}
	handle.waitCh <- nil
	store := runruntime.New(t.TempDir())
	record := agentrun.RunRecord{TicketID: "TKT-381", Role: agentrun.RoleImplementer, SessionID: "session-plain"}
	if err := store.Init(record, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	obs, err := New(Dependencies{Runtime: store}).Observe(context.Background(), agentrun.ObservationInput{
		Handle:  handle,
		Record:  record,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if obs.Result.Status != agentrun.StatusDone {
		t.Fatalf("unexpected observation: %#v", obs)
	}
	transcript, err := store.LoadTranscript("TKT-381")
	if err != nil {
		t.Fatalf("LoadTranscript() error = %v", err)
	}
	if len(transcript) == 0 || transcript[0].Text != "I am reading the code now" {
		t.Fatalf("expected plain stdout to be captured, got %#v", transcript)
	}
}

func TestObserverParsesStructuredReviewFromCodexJSONEvent(t *testing.T) {
	t.Parallel()

	handle := &fakeHandle{
		stdout: bytes.NewBufferString("{\"type\":\"item.completed\",\"item\":{\"id\":\"item_2\",\"type\":\"agent_message\",\"text\":\"REVIEW status=changes_required ticket=TKT-375 role=reviewer required_changes=\\\"add regression test\\\"\"}}\n"),
		stderr: bytes.NewReader(nil),
		waitCh: make(chan error, 1),
	}
	handle.waitCh <- nil

	obs, err := New().Observe(context.Background(), agentrun.ObservationInput{
		Handle:  handle,
		Record:  agentrun.RunRecord{TicketID: "TKT-375", Role: agentrun.RoleReviewer},
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if obs.Review == nil || obs.Review.Status != agentrun.ReviewChangesRequired {
		t.Fatalf("unexpected review observation: %#v", obs)
	}
}

func TestObserverFailsWhenProcessExitsAfterResultLine(t *testing.T) {
	t.Parallel()

	handle := &fakeHandle{
		stdout: bytes.NewBufferString("RESULT status=done ticket=TKT-381 role=implementer commit=abc123 tests=passed\n"),
		stderr: bytes.NewReader(nil),
		waitCh: make(chan error, 1),
	}
	handle.waitCh <- errors.New("process crashed after result")

	obs, err := New().Observe(context.Background(), agentrun.ObservationInput{
		Handle:  handle,
		Record:  agentrun.RunRecord{TicketID: "TKT-381", Role: agentrun.RoleImplementer},
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if obs.Result.Status != agentrun.StatusFailed || !strings.Contains(obs.Result.Reason, "process exited after RESULT line") {
		t.Fatalf("unexpected observation: %#v", obs)
	}
}

func TestObserverTreatsMalformedReviewLineAsChangesRequiredForReviewer(t *testing.T) {
	t.Parallel()

	handle := &fakeHandle{
		stdout: bytes.NewBufferString("REVIEW status=changes_required ticket=TKT-375 role=reviewer\n"),
		stderr: bytes.NewReader(nil),
		waitCh: make(chan error, 1),
	}
	handle.waitCh <- nil

	obs, err := New().Observe(context.Background(), agentrun.ObservationInput{
		Handle:  handle,
		Record:  agentrun.RunRecord{TicketID: "TKT-375", Role: agentrun.RoleReviewer},
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if obs.Review == nil || obs.Review.Status != agentrun.ReviewChangesRequired || !strings.Contains(obs.Review.RequiredChanges, "malformed REVIEW line") {
		t.Fatalf("unexpected review observation: %#v", obs)
	}
}

func TestObserverFailsWhenResultLineMissing(t *testing.T) {
	t.Parallel()

	handle := &fakeHandle{
		stdout: bytes.NewBufferString("no structured result here\n"),
		stderr: bytes.NewReader(nil),
		waitCh: make(chan error, 1),
	}
	handle.waitCh <- nil

	obs, err := New().Observe(context.Background(), agentrun.ObservationInput{
		Handle:  handle,
		Record:  agentrun.RunRecord{TicketID: "TKT-381", Role: agentrun.RoleImplementer},
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if obs.Result.Status != agentrun.StatusFailed {
		t.Fatalf("unexpected observation: %#v", obs)
	}
	if obs.Result.TicketID != "TKT-381" {
		t.Fatalf("unexpected observation: %#v", obs)
	}
}

func TestObserverFailsWhenResultLineMalformed(t *testing.T) {
	t.Parallel()

	handle := &fakeHandle{
		stdout: bytes.NewBufferString("RESULT status=done ticket=TKT-381\n"),
		stderr: bytes.NewReader(nil),
		waitCh: make(chan error, 1),
	}
	handle.waitCh <- nil

	obs, err := New().Observe(context.Background(), agentrun.ObservationInput{
		Handle:  handle,
		Record:  agentrun.RunRecord{TicketID: "TKT-381", Role: agentrun.RoleImplementer},
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if obs.Result.Status != agentrun.StatusFailed {
		t.Fatalf("unexpected observation: %#v", obs)
	}
	if obs.Result.Reason == "" {
		t.Fatalf("expected malformed reason in %#v", obs)
	}
}

func TestObserverTimesOutAndKillsSilentRun(t *testing.T) {
	t.Parallel()

	handle := &fakeHandle{
		stdout: bytes.NewReader(nil),
		stderr: bytes.NewReader(nil),
		waitCh: make(chan error, 1),
	}

	obs, err := New().Observe(context.Background(), agentrun.ObservationInput{
		Handle:  handle,
		Record:  agentrun.RunRecord{TicketID: "TKT-381", Role: agentrun.RoleImplementer},
		Timeout: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if !obs.TimedOut {
		t.Fatalf("expected timeout observation: %#v", obs)
	}
	if obs.Result.Status != agentrun.StatusFailed {
		t.Fatalf("unexpected observation: %#v", obs)
	}
	if !handle.killed {
		t.Fatalf("expected timeout to kill process")
	}
}

func TestObserverPersistsVisibleTranscriptAndStatusForActiveRun(t *testing.T) {
	t.Parallel()

	store := runruntime.New(t.TempDir())
	handle := &fakeHandle{
		stdout: bytes.NewBufferString("{\"type\":\"turn.started\"}\n{\"type\":\"item.completed\",\"item\":{\"id\":\"item_1\",\"type\":\"agent_message\",\"text\":\"PLAN ticket=TKT-381 steps=3\"}}\n{\"type\":\"item.completed\",\"item\":{\"id\":\"item_2\",\"type\":\"agent_message\",\"text\":\"STEP ticket=TKT-381 index=1 status=in_progress title=\\\"inspect repo\\\"\"}}\n{\"type\":\"item.completed\",\"item\":{\"id\":\"item_3\",\"type\":\"agent_message\",\"text\":\"RESULT status=done ticket=TKT-381 role=implementer commit=abc123 tests=passed\"}}\n"),
		stderr: bytes.NewReader(nil),
		waitCh: make(chan error, 1),
	}
	handle.waitCh <- nil

	record := agentrun.RunRecord{TicketID: "TKT-381", Role: agentrun.RoleImplementer, SessionID: "session-1"}
	if err := store.Init(record, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	obs, err := New(Dependencies{Runtime: store, Now: func() time.Time { return time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC) }}).Observe(context.Background(), agentrun.ObservationInput{
		Handle:  handle,
		Record:  record,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if obs.Result.Status != agentrun.StatusDone {
		t.Fatalf("unexpected observation: %#v", obs)
	}
	status, ok, err := store.LoadStatus("TKT-381")
	if err != nil || !ok {
		t.Fatalf("LoadStatus() ok=%v err=%v", ok, err)
	}
	if status.PlannedSteps != 3 || status.CurrentStep != 1 || status.LastMarker != "RESULT" {
		t.Fatalf("unexpected status snapshot: %#v", status)
	}
	transcript, err := store.LoadTranscript("TKT-381")
	if err != nil {
		t.Fatalf("LoadTranscript() error = %v", err)
	}
	if len(transcript) < 3 {
		t.Fatalf("unexpected transcript: %#v", transcript)
	}
}

func TestObserverReplaysRealCodexGoldenStreamIncrementally(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "codex_exec_golden.jsonl"))
	if err != nil {
		t.Fatalf("read golden fixture: %v", err)
	}
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()
	waitCh := make(chan error, 1)
	handle := &fakeHandle{
		stdout: stdoutR,
		stderr: stderrR,
		waitCh: waitCh,
	}
	store := runruntime.New(t.TempDir())
	record := agentrun.RunRecord{TicketID: "TKT-GOLDEN", Role: agentrun.RoleImplementer, SessionID: "golden-session"}
	if err := store.Init(record, "golden prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	go func() {
		defer stdoutW.Close()
		defer stderrW.Close()
		for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte("\n")) {
			_, _ = stdoutW.Write(append(line, '\n'))
			time.Sleep(5 * time.Millisecond)
		}
		waitCh <- nil
	}()

	obs, err := New(Dependencies{Runtime: store}).Observe(context.Background(), agentrun.ObservationInput{
		Handle:  handle,
		Record:  record,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if obs.Result.Status != agentrun.StatusDone || obs.Result.CommitSHA != "deadbeef" {
		t.Fatalf("unexpected observation: %#v", obs)
	}
	status, ok, err := store.LoadStatus("TKT-GOLDEN")
	if err != nil || !ok {
		t.Fatalf("LoadStatus() ok=%v err=%v", ok, err)
	}
	if status.PlannedSteps != 2 || status.CurrentStep != 1 || status.LastMarker != "RESULT" {
		t.Fatalf("unexpected status from golden stream: %#v", status)
	}
	transcript, err := store.LoadTranscript("TKT-GOLDEN")
	if err != nil {
		t.Fatalf("LoadTranscript() error = %v", err)
	}
	if len(transcript) != 4 {
		t.Fatalf("unexpected transcript length from golden stream: %#v", transcript)
	}
	if transcript[0].Text != "PLAN ticket=TKT-GOLDEN steps=2" || transcript[3].Text != "RESULT status=done ticket=TKT-GOLDEN role=implementer commit=deadbeef tests=passed" {
		t.Fatalf("unexpected transcript contents: %#v", transcript)
	}
}

func TestObserverRecordsVisibleStatusMarkersAndIgnoresPlainStderrNoiseInTranscript(t *testing.T) {
	t.Parallel()

	store := runruntime.New(t.TempDir())
	handle := &fakeHandle{
		stdout: bytes.NewBufferString("{\"type\":\"item.completed\",\"item\":{\"id\":\"item_1\",\"type\":\"agent_message\",\"text\":\"PLAN ticket=TKT-390 steps=2\\nSTATUS ticket=TKT-390 phase=testing\\nRESULT status=done ticket=TKT-390 role=implementer commit=abc123 tests=passed\"}}\n"),
		stderr: bytes.NewBufferString("plain stderr noise\n"),
		waitCh: make(chan error, 1),
	}
	handle.waitCh <- nil

	record := agentrun.RunRecord{TicketID: "TKT-390", Role: agentrun.RoleImplementer, SessionID: "session-390"}
	if err := store.Init(record, "prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	obs, err := New(Dependencies{Runtime: store}).Observe(context.Background(), agentrun.ObservationInput{
		Handle:  handle,
		Record:  record,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if obs.Result.Status != agentrun.StatusDone {
		t.Fatalf("unexpected observation: %#v", obs)
	}
	status, ok, err := store.LoadStatus("TKT-390")
	if err != nil || !ok {
		t.Fatalf("LoadStatus() ok=%v err=%v", ok, err)
	}
	if status.CurrentPhase != "testing" || status.LastMarker != "RESULT" {
		t.Fatalf("unexpected status snapshot: %#v", status)
	}
	transcript, err := store.LoadTranscript("TKT-390")
	if err != nil {
		t.Fatalf("LoadTranscript() error = %v", err)
	}
	if len(transcript) != 3 {
		t.Fatalf("unexpected transcript: %#v", transcript)
	}
	for _, entry := range transcript {
		if strings.Contains(entry.Text, "stderr noise") {
			t.Fatalf("stderr noise leaked into transcript: %#v", transcript)
		}
	}
}
