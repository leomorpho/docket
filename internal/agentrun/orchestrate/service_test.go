package orchestrate

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/agentrun"
	"github.com/leomorpho/docket/internal/agentrun/monitor"
	runruntime "github.com/leomorpho/docket/internal/agentrun/runtime"
	runvalidate "github.com/leomorpho/docket/internal/agentrun/validate"
	"github.com/leomorpho/docket/internal/claim"
	"github.com/leomorpho/docket/internal/security"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/leomorpho/docket/internal/vcs"
	"github.com/leomorpho/docket/internal/workflow"
)

type recordingAdapter struct {
	mu     sync.Mutex
	starts []string
	spec   agentrun.RunSpec
	record agentrun.RunRecord
}

func (a *recordingAdapter) ID() string { return "recording" }

func (a *recordingAdapter) Start(ctx context.Context, spec agentrun.RunSpec) (agentrun.ProcessHandle, agentrun.RunRecord, error) {
	a.mu.Lock()
	a.starts = append(a.starts, spec.TicketID)
	a.mu.Unlock()
	a.spec = spec
	a.record = agentrun.RunRecord{
		TicketID:     spec.TicketID,
		Role:         spec.Role,
		Adapter:      a.ID(),
		RepoRoot:     spec.RepoRoot,
		WorktreePath: spec.WorktreePath,
		Branch:       spec.Branch,
		StartedAt:    "2026-03-19T12:00:00Z",
		SessionID:    "session-380",
	}
	return stubHandle{stdout: bytes.NewBufferString("")}, a.record, nil
}

type fakeMonitor struct {
	mu    sync.Mutex
	queue []agentrun.Observation
}

func (m *fakeMonitor) Observe(ctx context.Context, input agentrun.ObservationInput) (agentrun.Observation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.queue) == 0 {
		return agentrun.Observation{}, nil
	}
	obs := m.queue[0]
	m.queue = m.queue[1:]
	return obs, nil
}

type fakeValidator struct {
	reasons map[string][]string
}

func (v fakeValidator) Validate(ctx context.Context, input agentrun.ValidationInput) (agentrun.ValidationResult, error) {
	reasons := v.reasons[input.TicketID]
	return agentrun.ValidationResult{Accepted: len(reasons) == 0, Reasons: reasons}, nil
}

func (v fakeValidator) Finalize(ctx context.Context, input agentrun.ValidationInput) (agentrun.ValidationResult, error) {
	return v.Validate(ctx, input)
}

type fakeSelector struct {
	queue []agentrun.Selection
	idx   int
}

func (s *fakeSelector) Next(ctx context.Context) (agentrun.Selection, error) {
	if s.idx >= len(s.queue) {
		return agentrun.Selection{Found: false, Reason: "no runnable tickets remain"}, nil
	}
	selection := s.queue[s.idx]
	s.idx++
	return selection, nil
}

type stubHandle struct {
	stdout io.Reader
}

func (h stubHandle) Stdout() io.Reader { return h.stdout }
func (h stubHandle) Stderr() io.Reader { return bytes.NewReader(nil) }
func (h stubHandle) Wait() error       { return nil }
func (h stubHandle) Kill() error       { return nil }
func (h stubHandle) PID() int          { return 1 }

type streamingHandle struct {
	stdout *io.PipeReader
	stderr *io.PipeReader
	waitCh chan error
	killCh chan struct{}
	pid    int
}

func (h *streamingHandle) Stdout() io.Reader { return h.stdout }
func (h *streamingHandle) Stderr() io.Reader { return h.stderr }
func (h *streamingHandle) Wait() error       { return <-h.waitCh }
func (h *streamingHandle) Kill() error {
	select {
	case <-h.killCh:
	default:
		close(h.killCh)
	}
	return nil
}
func (h *streamingHandle) PID() int { return h.pid }

type streamBehavior func(spec agentrun.RunSpec, stdout, stderr *io.PipeWriter, handle *streamingHandle)

type streamingAdapter struct {
	mu        sync.Mutex
	starts    []string
	specs     []agentrun.RunSpec
	behaviors []streamBehavior
	nextPID   int
}

func (a *streamingAdapter) ID() string { return "streaming" }

func (a *streamingAdapter) Start(ctx context.Context, spec agentrun.RunSpec) (agentrun.ProcessHandle, agentrun.RunRecord, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.behaviors) == 0 {
		return nil, agentrun.RunRecord{}, fmt.Errorf("no streaming behavior configured")
	}
	behavior := a.behaviors[0]
	a.behaviors = a.behaviors[1:]
	a.starts = append(a.starts, spec.TicketID)
	a.specs = append(a.specs, spec)
	a.nextPID++
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()
	handle := &streamingHandle{
		stdout: stdoutR,
		stderr: stderrR,
		waitCh: make(chan error, 1),
		killCh: make(chan struct{}),
		pid:    9000 + a.nextPID,
	}
	record := agentrun.RunRecord{
		TicketID:     spec.TicketID,
		Role:         spec.Role,
		Adapter:      a.ID(),
		RepoRoot:     spec.RepoRoot,
		WorktreePath: spec.WorktreePath,
		Branch:       spec.Branch,
		StartedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:    fmt.Sprintf("stream-%d", a.nextPID),
	}
	go behavior(spec, stdoutW, stderrW, handle)
	return handle, record, nil
}

func successfulStreamBehavior(t *testing.T, ticketID string) streamBehavior {
	t.Helper()
	return func(spec agentrun.RunSpec, stdout, stderr *io.PipeWriter, handle *streamingHandle) {
		defer stdout.Close()
		defer stderr.Close()
		defer func() { handle.waitCh <- nil }()
		writeStreamLine(stdout, `{"type":"thread.started","thread_id":"fixture-thread"}`)
		time.Sleep(5 * time.Millisecond)
		writeStreamLine(stdout, fmt.Sprintf("{\"type\":\"item.completed\",\"item\":{\"id\":\"item_1\",\"type\":\"agent_message\",\"text\":\"PLAN ticket=%s steps=3\"}}", ticketID))
		time.Sleep(5 * time.Millisecond)
		writeStreamLine(stdout, fmt.Sprintf("{\"type\":\"item.completed\",\"item\":{\"id\":\"item_2\",\"type\":\"agent_message\",\"text\":\"STEP ticket=%s index=1 status=in_progress title=\\\"write failing test\\\"\"}}", ticketID))
		path := filepath.Join(spec.WorktreePath, "feature.txt")
		if err := os.WriteFile(path, []byte("smoke-ok\n"), 0o644); err != nil {
			writeStreamLine(stderr, err.Error())
			handle.waitCh <- err
			return
		}
		runGit(t, spec.WorktreePath, "add", ".")
		runGit(t, spec.WorktreePath, "commit", "-m", fmt.Sprintf("feat: %s\n\nTicket: %s", ticketID, ticketID))
		sha := strings.TrimSpace(runGitOutput(t, spec.WorktreePath, "rev-parse", "HEAD"))
		time.Sleep(5 * time.Millisecond)
		writeStreamLine(stdout, fmt.Sprintf("{\"type\":\"item.completed\",\"item\":{\"id\":\"item_3\",\"type\":\"agent_message\",\"text\":\"STEP ticket=%s index=1 status=done title=\\\"write failing test\\\"\\nRESULT status=done ticket=%s role=implementer commit=%s tests=passed\"}}", ticketID, ticketID, sha))
	}
}

func hangingStreamBehavior(ticketID string) streamBehavior {
	return func(spec agentrun.RunSpec, stdout, stderr *io.PipeWriter, handle *streamingHandle) {
		defer stdout.Close()
		defer stderr.Close()
		writeStreamLine(stdout, fmt.Sprintf("{\"type\":\"item.completed\",\"item\":{\"id\":\"item_1\",\"type\":\"agent_message\",\"text\":\"PLAN ticket=%s steps=4\"}}", ticketID))
		time.Sleep(5 * time.Millisecond)
		writeStreamLine(stdout, fmt.Sprintf("{\"type\":\"item.completed\",\"item\":{\"id\":\"item_2\",\"type\":\"agent_message\",\"text\":\"STEP ticket=%s index=1 status=in_progress title=\\\"inspect repo\\\"\"}}", ticketID))
		<-handle.killCh
		handle.waitCh <- fmt.Errorf("killed")
	}
}

func writeStreamLine(w *io.PipeWriter, line string) {
	_, _ = io.WriteString(w, line+"\n")
}

func TestServiceStartImplementerCreatesManagedWorktreeAndPersistsRunRecord(t *testing.T) {
	t.Parallel()

	repoRoot := buildGitRepoForOrchestrationTest(t)
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store := local.New(repoRoot)
	if err := store.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-380",
		Seq:         380,
		Title:       "Implement runner",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedBy:   "human:test",
		Description: "Build the one ticket runner",
		AC:          []ticket.AcceptanceCriterion{{Description: "Use test-driven development"}},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	adapter := &recordingAdapter{}
	namespace := security.NewRepoNamespaceStore(filepath.Join(t.TempDir(), "home"))
	service := New(Dependencies{
		RepoRoot:  repoRoot,
		Actor:     "agent:test",
		Store:     store,
		Workflow:  workflow.NewManager(store, vcs.NewGitProvider(repoRoot), claim.NewLocalClaimManager(repoRoot)),
		Namespace: namespace,
		Adapter:   adapter,
	})

	started, err := service.StartImplementer(context.Background(), "TKT-380")
	if err != nil {
		t.Fatalf("StartImplementer() error = %v", err)
	}
	defer started.Handle.Kill()

	if started.Record.TicketID != "TKT-380" {
		t.Fatalf("started record = %#v", started.Record)
	}
	if started.WorktreePath == repoRoot {
		t.Fatalf("worktree path should not point at primary checkout: %s", started.WorktreePath)
	}
	if !strings.Contains(started.Branch, "docket/TKT-380") {
		t.Fatalf("unexpected branch %q", started.Branch)
	}
	if adapter.spec.WorktreePath != started.WorktreePath {
		t.Fatalf("adapter spec worktree = %q, want %q", adapter.spec.WorktreePath, started.WorktreePath)
	}
	for _, want := range []string{
		"Work only ticket TKT-380 in this run.",
		"Use test-driven development.",
		"Title: Implement runner",
		"Description: Build the one ticket runner",
		"Acceptance Criteria:",
		"- Use test-driven development",
		"Ticket: TKT-380",
		"RESULT status=done ticket=TKT-380 role=implementer commit=<sha> tests=passed",
	} {
		if !strings.Contains(adapter.spec.Prompt, want) {
			t.Fatalf("prompt missing %q in %q", want, adapter.spec.Prompt)
		}
	}

	recordPath := agentrun.RunRecordPath(repoRoot, "TKT-380")
	raw, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read run record: %v", err)
	}
	if !strings.Contains(string(raw), `"session_id": "session-380"`) {
		t.Fatalf("persisted record missing session id: %s", string(raw))
	}

	runManifest, ok, err := namespace.GetRunManifest(repoRoot, "TKT-380")
	if err != nil || !ok {
		t.Fatalf("GetRunManifest() ok=%v err=%v", ok, err)
	}
	if runManifest.Branch != "docket/TKT-380" {
		t.Fatalf("unexpected run manifest: %#v", runManifest)
	}
}

func TestServiceRunTicketUsesMonitorAndValidator(t *testing.T) {
	t.Parallel()

	repoRoot := buildGitRepoForOrchestrationTest(t)
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store := local.New(repoRoot)
	if err := store.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-381",
		Seq:         381,
		Title:       "Monitor path",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
		UpdatedAt:   time.Now().UTC().Truncate(time.Second),
		CreatedBy:   "human:test",
		Description: "desc",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	adapter := &recordingAdapter{}
	service := New(Dependencies{
		RepoRoot:  repoRoot,
		Actor:     "agent:test",
		Store:     store,
		Workflow:  workflow.NewManager(store, vcs.NewGitProvider(repoRoot), claim.NewLocalClaimManager(repoRoot)),
		Namespace: security.NewRepoNamespaceStore(filepath.Join(t.TempDir(), "home")),
		Adapter:   adapter,
		Monitor: &fakeMonitor{queue: []agentrun.Observation{
			{Result: agentrun.Result{Status: agentrun.StatusDone, TicketID: "TKT-381", Role: agentrun.RoleImplementer, CommitSHA: "abc123", Tests: "passed"}},
		}},
		Validator: fakeValidator{},
	})

	summary, err := service.RunTicket(context.Background(), "TKT-381")
	if err != nil {
		t.Fatalf("RunTicket() error = %v", err)
	}
	if summary.Status != agentrun.StatusDone {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}

func TestServiceRunTicketCleansRuntimeWhenValidatorRejectsSuccessfulRun(t *testing.T) {
	t.Parallel()

	repoRoot := buildGitRepoForOrchestrationTest(t)
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	if err := store.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-377",
		Seq:         377,
		Title:       "Validation reject",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "desc",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	runtimeStore := runruntime.New(repoRoot)
	service := New(Dependencies{
		RepoRoot:  repoRoot,
		Actor:     "agent:test",
		Store:     store,
		Workflow:  workflow.NewManager(store, vcs.NewGitProvider(repoRoot), claim.NewLocalClaimManager(repoRoot)),
		Namespace: security.NewRepoNamespaceStore(filepath.Join(t.TempDir(), "home")),
		Adapter:   &recordingAdapter{},
		Monitor: &fakeMonitor{queue: []agentrun.Observation{
			{Result: agentrun.Result{Status: agentrun.StatusDone, TicketID: "TKT-377", Role: agentrun.RoleImplementer, CommitSHA: "abc123", Tests: "passed"}},
		}},
		Validator: fakeValidator{reasons: map[string][]string{
			"TKT-377": {"acceptance command failed"},
		}},
		Runtime: runtimeStore,
	})

	summary, err := service.RunTicket(context.Background(), "TKT-377")
	if err != nil {
		t.Fatalf("RunTicket() error = %v", err)
	}
	if summary.Status != agentrun.StatusFailed || !strings.Contains(summary.Reason, "acceptance command failed") {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if _, ok, err := runtimeStore.LoadStatus("TKT-377"); err != nil || ok {
		t.Fatalf("expected runtime cleanup after validator rejection, ok=%v err=%v", ok, err)
	}
}

func TestServiceRunNextExecutesSeriallyAndStopsOnBlockingOutcome(t *testing.T) {
	t.Parallel()

	repoRoot := buildGitRepoForOrchestrationTest(t)
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	for _, id := range []string{"TKT-376", "TKT-375"} {
		if err := store.CreateTicket(context.Background(), &ticket.Ticket{
			ID:          id,
			Seq:         376,
			Title:       id,
			State:       ticket.State("todo"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "desc",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}

	adapter := &recordingAdapter{}
	service := New(Dependencies{
		RepoRoot:  repoRoot,
		Actor:     "agent:test",
		Store:     store,
		Workflow:  workflow.NewManager(store, vcs.NewGitProvider(repoRoot), claim.NewLocalClaimManager(repoRoot)),
		Namespace: security.NewRepoNamespaceStore(filepath.Join(t.TempDir(), "home")),
		Adapter:   adapter,
		Monitor: &fakeMonitor{queue: []agentrun.Observation{
			{Result: agentrun.Result{Status: agentrun.StatusDone, TicketID: "TKT-376", Role: agentrun.RoleImplementer, CommitSHA: "abc123", Tests: "passed"}},
			{Result: agentrun.Result{Status: agentrun.StatusStuck, TicketID: "TKT-375", Role: agentrun.RoleImplementer, Reason: "blocked by review findings"}},
		}},
		Validator: fakeValidator{reasons: map[string][]string{
			"TKT-375": {"blocked by review findings"},
		}},
		Selector: &fakeSelector{queue: []agentrun.Selection{
			{Found: true, TicketID: "TKT-376", Reason: "first"},
			{Found: true, TicketID: "TKT-375", Reason: "second"},
		}},
	})

	summary, err := service.RunNext(context.Background())
	if err != nil {
		t.Fatalf("RunNext() error = %v", err)
	}
	if len(summary.Runs) != 2 {
		t.Fatalf("expected 2 runs, got %#v", summary)
	}
	if summary.StopReason != "blocked by review findings" {
		t.Fatalf("unexpected stop reason: %#v", summary)
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if got := strings.Join(adapter.starts, ","); got != "TKT-376,TKT-375" {
		t.Fatalf("unexpected start order: %s", got)
	}
}

func TestServiceRunNextReturnsSelectorReasonWhenNoRunnableTicketsRemain(t *testing.T) {
	t.Parallel()

	service := New(Dependencies{
		Selector: &fakeSelector{queue: nil},
	})

	summary, err := service.RunNext(context.Background())
	if err != nil {
		t.Fatalf("RunNext() error = %v", err)
	}
	if len(summary.Runs) != 0 || summary.StopReason != "no runnable tickets remain" {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}

func TestServiceRunTicketWithReviewerApprovedFinalizesSuccess(t *testing.T) {
	t.Parallel()

	repoRoot := buildGitRepoForOrchestrationTest(t)
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store := local.New(repoRoot)
	if err := store.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-375",
		Seq:         375,
		Title:       "Review loop",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
		UpdatedAt:   time.Now().UTC().Truncate(time.Second),
		CreatedBy:   "human:test",
		Description: "desc",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	impl := &recordingAdapter{}
	reviewer := &recordingAdapter{}
	service := New(Dependencies{
		RepoRoot:  repoRoot,
		Actor:     "agent:test",
		Store:     store,
		Workflow:  workflow.NewManager(store, vcs.NewGitProvider(repoRoot), claim.NewLocalClaimManager(repoRoot)),
		Namespace: security.NewRepoNamespaceStore(filepath.Join(t.TempDir(), "home")),
		Adapter:   impl,
		Reviewer:  reviewer,
		Monitor: &fakeMonitor{queue: []agentrun.Observation{
			{Result: agentrun.Result{Status: agentrun.StatusDone, TicketID: "TKT-375", Role: agentrun.RoleImplementer, CommitSHA: "abc123", Tests: "passed"}},
			{Review: &agentrun.ReviewResult{Status: agentrun.ReviewApproved, TicketID: "TKT-375", Role: agentrun.RoleReviewer}},
		}},
		Validator: fakeValidator{},
	})

	summary, err := service.RunTicket(context.Background(), "TKT-375")
	if err != nil {
		t.Fatalf("RunTicket() error = %v", err)
	}
	if summary.Status != agentrun.StatusDone {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}

func TestServiceRunTicketWithReviewerChangesRequiredRunsOneFixLoop(t *testing.T) {
	t.Parallel()

	repoRoot := buildGitRepoForOrchestrationTest(t)
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store := local.New(repoRoot)
	if err := store.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-375",
		Seq:         375,
		Title:       "Review loop",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
		UpdatedAt:   time.Now().UTC().Truncate(time.Second),
		CreatedBy:   "human:test",
		Description: "desc",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	impl := &recordingAdapter{}
	reviewer := &recordingAdapter{}
	service := New(Dependencies{
		RepoRoot:  repoRoot,
		Actor:     "agent:test",
		Store:     store,
		Workflow:  workflow.NewManager(store, vcs.NewGitProvider(repoRoot), claim.NewLocalClaimManager(repoRoot)),
		Namespace: security.NewRepoNamespaceStore(filepath.Join(t.TempDir(), "home")),
		Adapter:   impl,
		Reviewer:  reviewer,
		Monitor: &fakeMonitor{queue: []agentrun.Observation{
			{Result: agentrun.Result{Status: agentrun.StatusDone, TicketID: "TKT-375", Role: agentrun.RoleImplementer, CommitSHA: "abc123", Tests: "passed"}},
			{Review: &agentrun.ReviewResult{Status: agentrun.ReviewChangesRequired, TicketID: "TKT-375", Role: agentrun.RoleReviewer, RequiredChanges: "add regression test"}},
			{Result: agentrun.Result{Status: agentrun.StatusDone, TicketID: "TKT-375", Role: agentrun.RoleImplementer, CommitSHA: "def456", Tests: "passed"}},
			{Review: &agentrun.ReviewResult{Status: agentrun.ReviewApproved, TicketID: "TKT-375", Role: agentrun.RoleReviewer}},
		}},
		Validator: fakeValidator{},
	})

	summary, err := service.RunTicket(context.Background(), "TKT-375")
	if err != nil {
		t.Fatalf("RunTicket() error = %v", err)
	}
	if summary.Status != agentrun.StatusDone {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	if got := strings.Join(impl.starts, ","); got != "TKT-375,TKT-375" {
		t.Fatalf("expected one fresh fix loop, got implementer starts %s", got)
	}
}

func TestServiceRunTicketStopsAfterSingleFixReviewLoopWhenReviewerStillRequestsChanges(t *testing.T) {
	t.Parallel()

	repoRoot := buildGitRepoForOrchestrationTest(t)
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store := local.New(repoRoot)
	if err := store.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-375",
		Seq:         375,
		Title:       "Review loop",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
		UpdatedAt:   time.Now().UTC().Truncate(time.Second),
		CreatedBy:   "human:test",
		Description: "desc",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	impl := &recordingAdapter{}
	reviewer := &recordingAdapter{}
	service := New(Dependencies{
		RepoRoot:  repoRoot,
		Actor:     "agent:test",
		Store:     store,
		Workflow:  workflow.NewManager(store, vcs.NewGitProvider(repoRoot), claim.NewLocalClaimManager(repoRoot)),
		Namespace: security.NewRepoNamespaceStore(filepath.Join(t.TempDir(), "home")),
		Adapter:   impl,
		Reviewer:  reviewer,
		Monitor: &fakeMonitor{queue: []agentrun.Observation{
			{Result: agentrun.Result{Status: agentrun.StatusDone, TicketID: "TKT-375", Role: agentrun.RoleImplementer, CommitSHA: "abc123", Tests: "passed"}},
			{Review: &agentrun.ReviewResult{Status: agentrun.ReviewChangesRequired, TicketID: "TKT-375", Role: agentrun.RoleReviewer, RequiredChanges: "add regression test"}},
			{Result: agentrun.Result{Status: agentrun.StatusDone, TicketID: "TKT-375", Role: agentrun.RoleImplementer, CommitSHA: "def456", Tests: "passed"}},
			{Review: &agentrun.ReviewResult{Status: agentrun.ReviewChangesRequired, TicketID: "TKT-375", Role: agentrun.RoleReviewer, RequiredChanges: "still missing regression test"}},
		}},
		Validator: fakeValidator{},
	})

	summary, err := service.RunTicket(context.Background(), "TKT-375")
	if err != nil {
		t.Fatalf("RunTicket() error = %v", err)
	}
	if summary.Status != agentrun.StatusFailed || !strings.Contains(summary.Reason, "still missing regression test") {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	if got := strings.Join(impl.starts, ","); got != "TKT-375,TKT-375" {
		t.Fatalf("expected exactly one fix loop, got implementer starts %s", got)
	}
}

func TestServiceResumeTicketUsesHungRuntimeStateAndCleansUpOnSuccess(t *testing.T) {
	t.Parallel()

	repoRoot := buildGitRepoForOrchestrationTest(t)
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	if err := store.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-376",
		Seq:         376,
		Title:       "Resume run",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "desc",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	namespace := security.NewRepoNamespaceStore(filepath.Join(t.TempDir(), "home"))
	flow := workflow.NewManager(store, vcs.NewGitProvider(repoRoot), claim.NewLocalClaimManager(repoRoot))
	_, worktreePath, err := flow.StartTask(context.Background(), "TKT-376", "agent:test", ticket.DefaultConfig())
	if err != nil {
		t.Fatalf("StartTask() error = %v", err)
	}
	if err := namespace.RecordRunStart(repoRoot, "TKT-376", "agent:test", worktreePath, "docket/TKT-376", ""); err != nil {
		t.Fatalf("RecordRunStart() error = %v", err)
	}
	runtimeStore := runruntime.New(repoRoot)
	record := agentrun.RunRecord{
		TicketID:     "TKT-376",
		Role:         agentrun.RoleImplementer,
		Adapter:      "recording",
		RepoRoot:     repoRoot,
		WorktreePath: worktreePath,
		Branch:       "docket/TKT-376",
		StartedAt:    now.Format(time.RFC3339Nano),
		SessionID:    "old-session",
	}
	if err := runtimeStore.Init(record, "original prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := runtimeStore.AppendTranscript("TKT-376", runruntime.TranscriptEntry{At: now.Format(time.RFC3339Nano), Text: "PLAN ticket=TKT-376 steps=5"}); err != nil {
		t.Fatalf("AppendTranscript() error = %v", err)
	}
	if err := runtimeStore.WriteStatus(runruntime.StatusSnapshot{
		TicketID:          "TKT-376",
		SessionID:         "old-session",
		Active:            false,
		Hung:              true,
		PlannedSteps:      5,
		CurrentStep:       2,
		CurrentStepTitle:  "write test",
		InactivityTimeout: "10m0s",
	}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}

	adapter := &recordingAdapter{}
	service := New(Dependencies{
		RepoRoot:  repoRoot,
		Actor:     "agent:test",
		Store:     store,
		Workflow:  flow,
		Namespace: namespace,
		Adapter:   adapter,
		Monitor: &fakeMonitor{queue: []agentrun.Observation{
			{Result: agentrun.Result{Status: agentrun.StatusDone, TicketID: "TKT-376", Role: agentrun.RoleImplementer, CommitSHA: "abc123", Tests: "passed"}},
		}},
		Validator: fakeValidator{},
		Runtime:   runtimeStore,
	})

	summary, err := service.ResumeTicket(context.Background(), "TKT-376")
	if err != nil {
		t.Fatalf("ResumeTicket() error = %v", err)
	}
	if summary.Status != agentrun.StatusDone {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if _, ok, err := runtimeStore.LoadStatus("TKT-376"); err != nil || ok {
		t.Fatalf("expected runtime cleanup after successful resume, ok=%v err=%v", ok, err)
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if len(adapter.starts) != 1 || adapter.starts[0] != "TKT-376" {
		t.Fatalf("unexpected resume starts: %#v", adapter.starts)
	}
	if !strings.Contains(adapter.spec.Prompt, "Previous run hung before completion.") {
		t.Fatalf("resume prompt missing handoff context: %q", adapter.spec.Prompt)
	}
}

func TestServiceRunTicketFullLifecycleWithStreamedCodexOutput(t *testing.T) {
	t.Parallel()

	repoRoot := buildGitRepoForOrchestrationTest(t)
	cfg := ticket.DefaultConfig()
	if err := ticket.SaveConfig(repoRoot, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	if err := store.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-390",
		Seq:         390,
		Title:       "Lifecycle smoke",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "Create feature.txt in the repo root.",
		AC: []ticket.AcceptanceCriterion{
			{Description: "feature.txt exists", Run: "test -f feature.txt"},
		},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	namespace := security.NewRepoNamespaceStore(filepath.Join(t.TempDir(), "home"))
	workflowSvc := workflow.NewManager(store, vcs.NewGitProvider(repoRoot), claim.NewLocalClaimManager(repoRoot))
	runtimeStore := runruntime.New(repoRoot)
	adapter := &streamingAdapter{behaviors: []streamBehavior{
		successfulStreamBehavior(t, "TKT-390"),
	}}
	validator := runvalidate.New(runvalidate.Dependencies{
		RepoRoot: repoRoot,
		Store:    store,
		Workflow: workflowSvc,
	})
	service := New(Dependencies{
		RepoRoot:  repoRoot,
		Actor:     "agent:test",
		Store:     store,
		Workflow:  workflowSvc,
		Namespace: namespace,
		Adapter:   adapter,
		Monitor:   monitor.New(monitor.Dependencies{Runtime: runtimeStore}),
		Validator: validator,
		Runtime:   runtimeStore,
		Timeout:   2 * time.Second,
	})

	summary, err := service.RunTicket(context.Background(), "TKT-390")
	if err != nil {
		t.Fatalf("RunTicket() error = %v", err)
	}
	if summary.Status != agentrun.StatusDone {
		t.Fatalf("unexpected summary: %#v", summary)
	}

	tkt, err := store.GetTicket(context.Background(), "TKT-390")
	if err != nil {
		t.Fatalf("GetTicket() error = %v", err)
	}
	if tkt.State != ticket.State("in-review") {
		t.Fatalf("ticket state = %q, want in-review", tkt.State)
	}
	if len(tkt.LinkedCommits) == 0 {
		t.Fatalf("expected linked commit after finalize: %#v", tkt)
	}
	if _, ok, err := runtimeStore.LoadStatus("TKT-390"); err != nil || ok {
		t.Fatalf("expected runtime cleanup after success, ok=%v err=%v", ok, err)
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if len(adapter.specs) != 1 {
		t.Fatalf("unexpected adapter starts: %#v", adapter.starts)
	}
	if !strings.Contains(adapter.specs[0].Prompt, "PLAN ticket=TKT-390 steps=<N>") {
		t.Fatalf("prompt missing plan contract: %q", adapter.specs[0].Prompt)
	}
}

func TestServiceRunTicketHungThenResumeFullLifecycle(t *testing.T) {
	t.Parallel()

	repoRoot := buildGitRepoForOrchestrationTest(t)
	cfg := ticket.DefaultConfig()
	if err := ticket.SaveConfig(repoRoot, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	if err := store.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-391",
		Seq:         391,
		Title:       "Resume smoke",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "Create feature.txt in the repo root.",
		AC: []ticket.AcceptanceCriterion{
			{Description: "feature.txt exists", Run: "test -f feature.txt"},
		},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	namespace := security.NewRepoNamespaceStore(filepath.Join(t.TempDir(), "home"))
	workflowSvc := workflow.NewManager(store, vcs.NewGitProvider(repoRoot), claim.NewLocalClaimManager(repoRoot))
	runtimeStore := runruntime.New(repoRoot)
	adapter := &streamingAdapter{behaviors: []streamBehavior{
		hangingStreamBehavior("TKT-391"),
		successfulStreamBehavior(t, "TKT-391"),
	}}
	validator := runvalidate.New(runvalidate.Dependencies{
		RepoRoot: repoRoot,
		Store:    store,
		Workflow: workflowSvc,
	})
	service := New(Dependencies{
		RepoRoot:  repoRoot,
		Actor:     "agent:test",
		Store:     store,
		Workflow:  workflowSvc,
		Namespace: namespace,
		Adapter:   adapter,
		Monitor:   monitor.New(monitor.Dependencies{Runtime: runtimeStore}),
		Validator: validator,
		Runtime:   runtimeStore,
		Timeout:   100 * time.Millisecond,
	})

	first, err := service.RunTicket(context.Background(), "TKT-391")
	if err != nil {
		t.Fatalf("first RunTicket() error = %v", err)
	}
	if first.Status != agentrun.StatusFailed || !strings.Contains(first.Reason, "timed out waiting for additional Codex output") {
		t.Fatalf("unexpected first summary: %#v", first)
	}
	status, ok, err := runtimeStore.LoadStatus("TKT-391")
	if err != nil || !ok || !status.Hung {
		t.Fatalf("expected hung runtime state, ok=%v status=%#v err=%v", ok, status, err)
	}
	if status.PlannedSteps == 0 || status.CurrentStep == 0 {
		t.Fatalf("expected tracked progress before hang: %#v", status)
	}

	resumed, err := service.ResumeTicket(context.Background(), "TKT-391")
	if err != nil {
		t.Fatalf("ResumeTicket() error = %v", err)
	}
	if resumed.Status != agentrun.StatusDone {
		t.Fatalf("unexpected resumed summary: %#v", resumed)
	}
	if _, ok, err := runtimeStore.LoadStatus("TKT-391"); err != nil || ok {
		t.Fatalf("expected runtime cleanup after resume success, ok=%v err=%v", ok, err)
	}
	tkt, err := store.GetTicket(context.Background(), "TKT-391")
	if err != nil {
		t.Fatalf("GetTicket() error = %v", err)
	}
	if tkt.State != ticket.State("in-review") {
		t.Fatalf("ticket state = %q, want in-review", tkt.State)
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if len(adapter.specs) != 2 {
		t.Fatalf("expected two adapter runs, got %#v", adapter.specs)
	}
	if !strings.Contains(adapter.specs[1].Prompt, "Previous run hung before completion.") {
		t.Fatalf("resume prompt missing hung context: %q", adapter.specs[1].Prompt)
	}
}
