package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/leomorpho/docket/internal/claim"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/ticket"
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
	commitCalls       int
	mergeCalls        int
	deleteCalls       int
	removeCalls       int
}

func (m *MockVCS) CreateWorktree(ctx context.Context, ticketID, branch, path string) error {
	return m.createWorktreeErr
}
func (m *MockVCS) RemoveWorktree(ctx context.Context, path string) error {
	m.removeCalls++
	return nil
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
	return nil
}
func (m *MockVCS) MergeBranch(ctx context.Context, branch string) error {
	m.mergeCalls++
	return nil
}
func (m *MockVCS) DeleteBranch(ctx context.Context, branch string) error {
	m.deleteCalls++
	return nil
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
}
