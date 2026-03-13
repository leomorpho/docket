package workflow

import (
	"context"
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
	repoRoot string
}

func (m *MockVCS) CreateWorktree(ctx context.Context, ticketID, branch, path string) error {
	return nil
}
func (m *MockVCS) RemoveWorktree(ctx context.Context, path string) error { return nil }
func (m *MockVCS) GetAgentWorktreeDir(ctx context.Context, ticketID string) (string, error) {
	return "/tmp/wt-" + ticketID, nil
}
func (m *MockVCS) GetRepoRoot(ctx context.Context) (string, error)       { return "/tmp/repo", nil }
func (m *MockVCS) CommitAll(ctx context.Context, path, msg string) error { return nil }
func (m *MockVCS) MergeBranch(ctx context.Context, branch string) error  { return nil }
func (m *MockVCS) DeleteBranch(ctx context.Context, branch string) error { return nil }

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
		return &claim.ClaimMetadata{AgentID: agent}, nil
	}
	return nil, nil
}

func TestWorkflowStartTask(t *testing.T) {
	s := &MockStore{t: &ticket.Ticket{ID: "TKT-001", State: "todo"}}
	v := &MockVCS{}
	c := &MockClaim{claims: make(map[string]string)}
	mgr := NewManager(s, v, c)
	cfg := ticket.DefaultConfig()

	res, wtPath, err := mgr.StartTask(context.Background(), "TKT-001", "agent-1", cfg)
	if err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}
	if wtPath == "" {
		t.Fatal("expected worktree path to be returned")
	}

	if res.State != "in-progress" {
		t.Errorf("expected state in-progress, got %s", res.State)
	}
	if res.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set")
	}
	if c.claims["TKT-001"] != "agent-1" {
		t.Errorf("expected TKT-001 to be claimed by agent-1, got %s", c.claims["TKT-001"])
	}
}

func TestWorkflowFinishTask(t *testing.T) {
	s := &MockStore{t: &ticket.Ticket{ID: "TKT-001", State: "in-progress"}}
	v := &MockVCS{}
	c := &MockClaim{claims: map[string]string{"TKT-001": "agent-1"}}
	mgr := NewManager(s, v, c)
	cfg := ticket.DefaultConfig()

	// Allow in-progress -> done
	st := cfg.States["in-progress"]
	st.Next = append(st.Next, "done")
	cfg.States["in-progress"] = st

	res, err := mgr.FinishTask(context.Background(), "TKT-001", cfg)
	if err != nil {
		t.Fatalf("FinishTask failed: %v", err)
	}

	if res.State != "done" {
		t.Errorf("expected state done, got %s", res.State)
	}
	if res.CompletedAt.IsZero() {
		t.Error("expected CompletedAt to be set")
	}
	if _, ok := c.claims["TKT-001"]; ok {
		t.Error("expected claim to be released")
	}
}
