package workflow

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/claim"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/leomorpho/docket/internal/vcs"
)

// MockStore
type MockStore struct {
	t *ticket.Ticket
}

func (m *MockStore) CreateTicket(ctx context.Context, t *ticket.Ticket) error { return nil }
func (m *MockStore) UpdateTicket(ctx context.Context, t *ticket.Ticket) error { m.t = t; return nil }
func (m *MockStore) GetTicket(ctx context.Context, id string) (*ticket.Ticket, error) {
	return m.t, nil
}
func (m *MockStore) GetRaw(ctx context.Context, id string) (string, error) { return "", nil }
func (m *MockStore) ListTickets(ctx context.Context, f store.Filter) ([]*ticket.Ticket, error) {
	return nil, nil
}
func (m *MockStore) AddComment(ctx context.Context, id string, c ticket.Comment) error { return nil }
func (m *MockStore) LinkCommit(ctx context.Context, id string, sha string) error       { return nil }
func (m *MockStore) NextID(ctx context.Context) (id string, seq int, err error)        { return "", 0, nil }
func (m *MockStore) Validate(ctx context.Context, id string) ([]store.ValidationError, error) {
	return nil, nil
}

// MockVCS
type MockVCS struct {
	repoRoot          string
	worktreePath      string
	getWorktreeDirErr error
	createWorktreeErr error
	removeWorktreeErr error
	deleteBranchErr   error
	commitCalls       int
	mergeCalls        int
	deleteCalls       int
	removeCalls       int
	ops               []string
}

func (m *MockVCS) CreateWorktree(ctx context.Context, ticketID, branch, path string) error {
	return m.createWorktreeErr
}
func (m *MockVCS) RemoveWorktree(ctx context.Context, path string) error {
	m.removeCalls++
	m.ops = append(m.ops, "remove")
	return m.removeWorktreeErr
}
func (m *MockVCS) GetAgentWorktreeDir(ctx context.Context, ticketID string) (string, error) {
	if m.getWorktreeDirErr != nil {
		return "", m.getWorktreeDirErr
	}
	if m.worktreePath != "" {
		return m.worktreePath, nil
	}
	return "/tmp/wt-" + ticketID, nil
}
func (m *MockVCS) GetRepoRoot(ctx context.Context) (string, error) {
	if m.repoRoot != "" {
		return m.repoRoot, nil
	}
	return "/tmp/repo", nil
}
func (m *MockVCS) CommitAll(ctx context.Context, path, msg string) error {
	m.commitCalls++
	m.ops = append(m.ops, "commit")
	return nil
}
func (m *MockVCS) MergeBranch(ctx context.Context, branch string) error {
	m.mergeCalls++
	m.ops = append(m.ops, "merge")
	return nil
}
func (m *MockVCS) DeleteBranch(ctx context.Context, branch string) error {
	m.deleteCalls++
	m.ops = append(m.ops, "delete")
	return m.deleteBranchErr
}

// MockClaim
type MockClaim struct {
	claims map[string]string
}

func (m *MockClaim) Claim(ctx context.Context, ticketID, worktreePath, agentID string) error {
	m.claims[ticketID] = agentID
	return nil
}
func (m *MockClaim) Release(ctx context.Context, ticketID string) error {
	delete(m.claims, ticketID)
	return nil
}
func (m *MockClaim) GetClaim(ctx context.Context, ticketID string) (*claim.ClaimMetadata, error) {
	if agent, ok := m.claims[ticketID]; ok {
		return &claim.ClaimMetadata{AgentID: agent, Worktree: "/tmp/wt-" + ticketID}, nil
	}
	return nil, nil
}

func TestWorkflowStartTask(t *testing.T) {
	s := &MockStore{t: &ticket.Ticket{ID: "TKT-001", State: "todo"}}
	v := &MockVCS{}
	c := &MockClaim{claims: make(map[string]string)}
	mgr := NewManager(s, v, c)
	cfg := ticket.DefaultConfig()

	res, wtPath, err := mgr.StartTask(context.Background(), "TKT-001", "agent:1", cfg)
	if err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}
	if wtPath == "" {
		t.Fatal("expected worktree path to be returned")
	}
	if wtPath == "/tmp/repo" {
		t.Fatalf("expected dedicated worktree path for agent-managed start, got repo root %s", wtPath)
	}

	if res.State != "in-progress" {
		t.Errorf("expected state in-progress, got %s", res.State)
	}
	if res.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set")
	}
	if c.claims["TKT-001"] != "agent:1" {
		t.Errorf("expected TKT-001 to be claimed by agent:1, got %s", c.claims["TKT-001"])
	}
}

func TestWorkflowStartTask_AgentRequiresDedicatedWorktree(t *testing.T) {
	cfg := ticket.DefaultConfig()

	t.Run("fails when worktree path lookup fails", func(t *testing.T) {
		s := &MockStore{t: &ticket.Ticket{ID: "TKT-001", State: "todo"}}
		v := &MockVCS{getWorktreeDirErr: errors.New("no cache dir")}
		c := &MockClaim{claims: make(map[string]string)}
		mgr := NewManager(s, v, c)

		_, _, err := mgr.StartTask(context.Background(), "TKT-001", "agent:test", cfg)
		if err == nil {
			t.Fatal("expected error for missing dedicated worktree path")
		}
	})

	t.Run("fails when worktree creation fails", func(t *testing.T) {
		s := &MockStore{t: &ticket.Ticket{ID: "TKT-001", State: "todo"}}
		v := &MockVCS{createWorktreeErr: errors.New("git worktree add failed")}
		c := &MockClaim{claims: make(map[string]string)}
		mgr := NewManager(s, v, c)

		_, _, err := mgr.StartTask(context.Background(), "TKT-001", "agent:test", cfg)
		if err == nil {
			t.Fatal("expected error for failed worktree creation")
		}
	})

	t.Run("human flow still falls back to repo root", func(t *testing.T) {
		s := &MockStore{t: &ticket.Ticket{ID: "TKT-001", State: "todo"}}
		v := &MockVCS{
			repoRoot:          "/tmp/repo",
			createWorktreeErr: errors.New("git worktree add failed"),
		}
		c := &MockClaim{claims: make(map[string]string)}
		mgr := NewManager(s, v, c)

		_, wtPath, err := mgr.StartTask(context.Background(), "TKT-001", "human:test", cfg)
		if err != nil {
			t.Fatalf("unexpected error for human fallback: %v", err)
		}
		if wtPath != "/tmp/repo" {
			t.Fatalf("expected repo-root fallback for human flow, got %s", wtPath)
		}
	})
}

func TestWorkflowFinishTask(t *testing.T) {
	s := &MockStore{t: &ticket.Ticket{ID: "TKT-001", State: "in-progress"}}
	v := &MockVCS{}
	c := &MockClaim{claims: map[string]string{"TKT-001": "agent-1"}}
	mgr := NewManager(s, v, c)
	cfg := ticket.DefaultConfig()

	res, err := mgr.FinishTask(context.Background(), "TKT-001", cfg)
	if err != nil {
		t.Fatalf("FinishTask failed: %v", err)
	}

	if res.State != "in-review" {
		t.Errorf("expected state in-review, got %s", res.State)
	}
	if !res.CompletedAt.IsZero() {
		t.Error("expected CompletedAt to remain unset at in-review")
	}
	if _, ok := c.claims["TKT-001"]; ok {
		t.Error("expected claim to be released")
	}
	if v.commitCalls != 1 || v.mergeCalls != 1 || v.removeCalls != 1 || v.deleteCalls != 1 {
		t.Fatalf("expected merge-back lifecycle once, got commit=%d merge=%d remove=%d delete=%d", v.commitCalls, v.mergeCalls, v.removeCalls, v.deleteCalls)
	}
	if got, want := strings.Join(v.ops, ","), "commit,merge,remove,delete"; got != want {
		t.Fatalf("expected VCS lifecycle %s, got %s", want, got)
	}
}

func TestWorkflowFinishTask_FailsWhenWorktreeCleanupFails(t *testing.T) {
	s := &MockStore{t: &ticket.Ticket{ID: "TKT-001", State: "in-progress"}}
	v := &MockVCS{removeWorktreeErr: errors.New("prune failed")}
	c := &MockClaim{claims: map[string]string{"TKT-001": "agent-1"}}
	mgr := NewManager(s, v, c)
	cfg := ticket.DefaultConfig()

	_, err := mgr.FinishTask(context.Background(), "TKT-001", cfg)
	if err == nil {
		t.Fatal("expected cleanup failure")
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "cleanup merged worktree") {
		t.Fatalf("expected cleanup error, got %v", err)
	}
	if s.t.State != "in-progress" {
		t.Fatalf("expected ticket state unchanged on cleanup failure, got %s", s.t.State)
	}
	if _, ok := c.claims["TKT-001"]; !ok {
		t.Fatal("expected claim to remain when cleanup fails")
	}
	if v.deleteCalls != 0 {
		t.Fatalf("expected branch deletion to be skipped after cleanup failure, got %d calls", v.deleteCalls)
	}
}

func TestWorkflowFinishTask_FailsWhenBranchDeletionFails(t *testing.T) {
	s := &MockStore{t: &ticket.Ticket{ID: "TKT-001", State: "in-progress"}}
	v := &MockVCS{deleteBranchErr: errors.New("branch locked")}
	c := &MockClaim{claims: map[string]string{"TKT-001": "agent-1"}}
	mgr := NewManager(s, v, c)
	cfg := ticket.DefaultConfig()

	_, err := mgr.FinishTask(context.Background(), "TKT-001", cfg)
	if err == nil {
		t.Fatal("expected branch deletion failure")
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "delete merged branch") {
		t.Fatalf("expected branch deletion error, got %v", err)
	}
	if s.t.State != "in-progress" {
		t.Fatalf("expected ticket state unchanged on branch deletion failure, got %s", s.t.State)
	}
	if _, ok := c.claims["TKT-001"]; !ok {
		t.Fatal("expected claim to remain when branch deletion fails")
	}
}

func TestWorkflowFinishTask_RealGitWorktreeCleanup(t *testing.T) {
	repoRoot := t.TempDir()
	runGitWorkflow(t, repoRoot, "init")
	runGitWorkflow(t, repoRoot, "config", "user.email", "test@example.com")
	runGitWorkflow(t, repoRoot, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed failed: %v", err)
	}
	runGitWorkflow(t, repoRoot, "add", "README.md")
	runGitWorkflow(t, repoRoot, "commit", "-m", "chore: seed")

	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	s := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-999",
		Seq:         999,
		Title:       "Real finish cleanup",
		State:       ticket.State("in-progress"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	v := vcs.NewGitProvider(repoRoot)
	c := claim.NewLocalClaimManager(repoRoot)
	mgr := NewManager(s, v, c)
	worktreePath := filepath.Join(t.TempDir(), "wt-TKT-999")
	branch := "docket/TKT-999"
	if err := v.CreateWorktree(context.Background(), "TKT-999", branch, worktreePath); err != nil {
		t.Fatalf("CreateWorktree failed: %v", err)
	}
	if err := c.Claim(context.Background(), "TKT-999", worktreePath, "agent:test"); err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(worktreePath, "feature.txt"), []byte("ticket work\n"), 0o644); err != nil {
		t.Fatalf("write worktree file failed: %v", err)
	}

	res, err := mgr.FinishTask(context.Background(), "TKT-999", ticket.DefaultConfig())
	if err != nil {
		t.Fatalf("FinishTask failed: %v", err)
	}
	if res.State != "in-review" {
		t.Fatalf("expected in-review state, got %s", res.State)
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("expected worktree path removed, got err=%v", err)
	}
	if out := runGitWorkflowOutput(t, repoRoot, "worktree", "list"); strings.Contains(out, worktreePath) {
		t.Fatalf("expected pruned worktree list to exclude %s, got %s", worktreePath, out)
	}
	if out := runGitWorkflowOutput(t, repoRoot, "branch", "--list", branch); strings.TrimSpace(out) != "" {
		t.Fatalf("expected merged branch %s deleted, got %q", branch, out)
	}
	mergedData, err := os.ReadFile(filepath.Join(repoRoot, "feature.txt"))
	if err != nil {
		t.Fatalf("expected merged file in repo root: %v", err)
	}
	if string(mergedData) != "ticket work\n" {
		t.Fatalf("unexpected merged file contents: %q", string(mergedData))
	}
	cl, err := c.GetClaim(context.Background(), "TKT-999")
	if err != nil {
		t.Fatalf("GetClaim failed: %v", err)
	}
	if cl != nil {
		t.Fatalf("expected claim released, got %#v", cl)
	}
}

func runGitWorkflow(t *testing.T, repoRoot string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
	}
}

func runGitWorkflowOutput(t *testing.T, repoRoot string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
	}
	return string(out)
}
