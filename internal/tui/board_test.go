package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/ticket"
)

type fakeBackend struct {
	tickets map[string]*ticket.Ticket

	nextID  string
	nextSeq int

	listErr   error
	getErr    error
	createErr error
	updateErr error
	nextErr   error
}

func newFakeBackend(ts ...*ticket.Ticket) *fakeBackend {
	m := make(map[string]*ticket.Ticket, len(ts))
	for _, t := range ts {
		m[t.ID] = cloneTicket(t)
	}
	return &fakeBackend{tickets: m, nextID: "TKT-999", nextSeq: 999}
}

func cloneTicket(t *ticket.Ticket) *ticket.Ticket {
	if t == nil {
		return nil
	}
	cp := *t
	cp.Labels = append([]string{}, t.Labels...)
	cp.BlockedBy = append([]string{}, t.BlockedBy...)
	cp.Blocks = append([]string{}, t.Blocks...)
	cp.LinkedCommits = append([]string{}, t.LinkedCommits...)
	cp.Plan = append([]ticket.PlanStep{}, t.Plan...)
	cp.AC = append([]ticket.AcceptanceCriterion{}, t.AC...)
	cp.Comments = append([]ticket.Comment{}, t.Comments...)
	return &cp
}

func (f *fakeBackend) CreateTicket(ctx context.Context, t *ticket.Ticket) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.tickets[t.ID] = cloneTicket(t)
	return nil
}

func (f *fakeBackend) UpdateTicket(ctx context.Context, t *ticket.Ticket) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	if _, ok := f.tickets[t.ID]; !ok {
		return errors.New("ticket not found")
	}
	f.tickets[t.ID] = cloneTicket(t)
	return nil
}

func (f *fakeBackend) GetTicket(ctx context.Context, id string) (*ticket.Ticket, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	t, ok := f.tickets[id]
	if !ok {
		return nil, nil
	}
	return cloneTicket(t), nil
}

func (f *fakeBackend) GetRaw(ctx context.Context, id string) (string, error) {
	return "", nil
}

func (f *fakeBackend) ListTickets(ctx context.Context, filter store.Filter) ([]*ticket.Ticket, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]*ticket.Ticket, 0, len(f.tickets))
	for _, t := range f.tickets {
		if !filter.IncludeArchived && t.State == "archived" {
			continue
		}
		out = append(out, cloneTicket(t))
	}
	return out, nil
}

func (f *fakeBackend) AddComment(ctx context.Context, id string, c ticket.Comment) error {
	return nil
}

func (f *fakeBackend) LinkCommit(ctx context.Context, id string, sha string) error {
	return nil
}

func (f *fakeBackend) NextID(ctx context.Context) (id string, seq int, err error) {
	if f.nextErr != nil {
		return "", 0, f.nextErr
	}
	return f.nextID, f.nextSeq, nil
}

func (f *fakeBackend) Validate(ctx context.Context, id string) ([]store.ValidationError, error) {
	return nil, nil
}

// newBoardModelWithDefaultCfg creates a board model using the default config
// (useful in tests that don't have a real .docket directory).
func newBoardModelWithDefaultCfg(backend store.Backend, actor string) BoardModel {
	cfg := ticket.DefaultConfig()
	cols, stateToColIdx, blockedColIdx := buildColumnsFromConfig(cfg)
	return BoardModel{
		repoRoot:      "/tmp",
		backend:       backend,
		actor:         actor,
		cfg:           cfg,
		columns:       cols,
		stateToColIdx: stateToColIdx,
		blockedColIdx: blockedColIdx,
	}
}

func TestTargetState(t *testing.T) {
	m := newBoardModelWithDefaultCfg(newFakeBackend(), "human:test")

	next, err := m.targetState(ticket.State("backlog"), 1)
	if err != nil {
		t.Fatalf("targetState(backlog,+1) err = %v", err)
	}
	if next != ticket.State("todo") {
		t.Fatalf("targetState(backlog,+1) = %s, want todo", next)
	}

	// In the default config, done can move right to archived on the board.
	next, err = m.targetState(ticket.State("done"), 1)
	if err != nil {
		t.Fatalf("targetState(done,+1) err = %v", err)
	}
	if next != ticket.State("archived") {
		t.Fatalf("targetState(done,+1) = %s, want archived", next)
	}
	// Archived is the last column in default config; it has no right neighbor.
	_, err = m.targetState(ticket.State("archived"), 1)
	if err == nil || !strings.Contains(err.Error(), "cannot transition right from archived") {
		t.Fatalf("expected archived->right error, got %v", err)
	}

	_, err = m.targetState(ticket.State("backlog"), -1)
	if err == nil || !strings.Contains(err.Error(), "cannot transition left from backlog") {
		t.Fatalf("expected backlog->left error, got %v", err)
	}
}

func TestRebuildColumnsBlockedAndSorted(t *testing.T) {
	now := time.Now().UTC()
	m := newBoardModelWithDefaultCfg(nil, "human:test")
	m.allTickets = []*ticket.Ticket{
		{ID: "TKT-003", State: ticket.State("todo"), Priority: 2, Title: "C", CreatedAt: now.Add(2 * time.Hour)},
		{ID: "TKT-002", State: ticket.State("todo"), Priority: 1, Title: "B", CreatedAt: now.Add(1 * time.Hour)},
		{ID: "TKT-001", State: ticket.State("done"), Priority: 5, Title: "A", BlockedBy: []string{"TKT-999"}, CreatedAt: now},
	}

	m.rebuildColumns("")

	todoIdx := m.stateToColIdx["todo"]
	blockedIdx := m.blockedColIdx

	if len(m.columns[todoIdx].tickets) != 2 {
		t.Fatalf("todo column count = %d, want 2", len(m.columns[todoIdx].tickets))
	}
	if got := m.columns[todoIdx].tickets[0].ID; got != "TKT-002" {
		t.Fatalf("todo first ticket = %s, want TKT-002", got)
	}

	if len(m.columns[blockedIdx].tickets) != 1 {
		t.Fatalf("blocked column count = %d, want 1", len(m.columns[blockedIdx].tickets))
	}
	if got := m.columns[blockedIdx].tickets[0].ID; got != "TKT-001" {
		t.Fatalf("blocked ticket = %s, want TKT-001", got)
	}
}

func TestUpdateStateCmdPersistsUsingFullTicket(t *testing.T) {
	cfg := ticket.DefaultConfig()
	now := time.Now().UTC().Add(-time.Hour)
	b := newFakeBackend(&ticket.Ticket{
		ID:          "TKT-001",
		State:       ticket.State("backlog"),
		Priority:    3,
		Title:       "Title",
		Labels:      []string{"bug"},
		Comments:    []ticket.Comment{{Author: "human:x", Body: "note", At: now}},
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:x",
		Description: "desc",
	})

	msg := updateStateCmd(b, "TKT-001", ticket.State("todo"), cfg)().(opMsg)
	if msg.err != nil {
		t.Fatalf("updateStateCmd err = %v", msg.err)
	}

	got := b.tickets["TKT-001"]
	if got.State != ticket.State("todo") {
		t.Fatalf("state = %s, want todo", got.State)
	}
	if len(got.Comments) != 1 {
		t.Fatalf("comments length = %d, want 1", len(got.Comments))
	}
	if !got.UpdatedAt.After(now) {
		t.Fatalf("updated_at not bumped: %s <= %s", got.UpdatedAt, now)
	}
}

func TestSwapPriorityCmdSwapsBothTickets(t *testing.T) {
	now := time.Now().UTC().Add(-time.Hour)
	b := newFakeBackend(
		&ticket.Ticket{ID: "TKT-001", Priority: 1, State: ticket.State("todo"), CreatedAt: now, UpdatedAt: now, CreatedBy: "human:x", Title: "A"},
		&ticket.Ticket{ID: "TKT-002", Priority: 2, State: ticket.State("todo"), CreatedAt: now, UpdatedAt: now, CreatedBy: "human:x", Title: "B"},
	)

	msg := swapPriorityCmd(b, "TKT-001", "TKT-002")().(opMsg)
	if msg.err != nil {
		t.Fatalf("swapPriorityCmd err = %v", msg.err)
	}
	if b.tickets["TKT-001"].Priority != 2 || b.tickets["TKT-002"].Priority != 1 {
		t.Fatalf("priorities not swapped: got %d/%d", b.tickets["TKT-001"].Priority, b.tickets["TKT-002"].Priority)
	}
}

func TestCreateTicketCmdDefaults(t *testing.T) {
	cfg := ticket.DefaultConfig()
	b := newFakeBackend()
	b.nextID = "TKT-010"
	b.nextSeq = 10

	msg := createTicketCmd(b, "human:tester", "From board", cfg)().(opMsg)
	if msg.err != nil {
		t.Fatalf("createTicketCmd err = %v", msg.err)
	}

	created := b.tickets["TKT-010"]
	if created == nil {
		t.Fatalf("expected created ticket")
	}
	if created.State != ticket.State(cfg.DefaultState) {
		t.Fatalf("state = %s, want %s", created.State, cfg.DefaultState)
	}
	if created.Priority != cfg.DefaultPriority {
		t.Fatalf("priority = %d, want %d", created.Priority, cfg.DefaultPriority)
	}
	if created.CreatedBy != "human:tester" {
		t.Fatalf("created_by = %s, want human:tester", created.CreatedBy)
	}
}

func TestMoveAndReorderBlockedColumnErrors(t *testing.T) {
	m := newBoardModelWithDefaultCfg(newFakeBackend(), "human:test")
	blockedIdx := m.blockedColIdx
	m.columns[blockedIdx].tickets = []*ticket.Ticket{{ID: "TKT-123", State: ticket.State("todo"), BlockedBy: []string{"TKT-001"}}}
	m.focusCol = blockedIdx
	m.focusRow = 0

	moveMsg := m.moveStateCmd(1)().(opMsg)
	if moveMsg.err == nil || !strings.Contains(moveMsg.err.Error(), "cannot move tickets from BLOCKED column") {
		t.Fatalf("move error = %v", moveMsg.err)
	}

	reorderMsg := m.reorderPriorityCmd(1)().(opMsg)
	if reorderMsg.err == nil || !strings.Contains(reorderMsg.err.Error(), "cannot reprioritize in BLOCKED column") {
		t.Fatalf("reorder error = %v", reorderMsg.err)
	}
}

func TestCreateTitleInputFlow(t *testing.T) {
	b := newFakeBackend()
	b.nextID = "TKT-011"
	b.nextSeq = 11

	m := newBoardModelWithDefaultCfg(b, "human:test")

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = model.(BoardModel)
	if !m.creatingTitle {
		t.Fatalf("expected creatingTitle true")
	}

	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	m = model.(BoardModel)
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'B'}})
	m = model.(BoardModel)
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = model.(BoardModel)
	if m.newTitle != "A" {
		t.Fatalf("newTitle = %q, want %q", m.newTitle, "A")
	}

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(BoardModel)
	if m.creatingTitle {
		t.Fatalf("expected create mode closed")
	}
	if cmd == nil {
		t.Fatalf("expected create command")
	}
	result := cmd().(opMsg)
	if result.err != nil {
		t.Fatalf("create op err = %v", result.err)
	}
	if b.tickets["TKT-011"] == nil {
		t.Fatalf("expected created ticket in backend")
	}
}

func TestLoadDetailCmd(t *testing.T) {
	now := time.Now().UTC()
	b := newFakeBackend(&ticket.Ticket{ID: "TKT-001", State: ticket.State("todo"), Priority: 1, Title: "Hello", CreatedAt: now, UpdatedAt: now, CreatedBy: "human"})

	msg := loadDetailCmd(b, "TKT-001")().(detailMsg)
	if msg.err != nil {
		t.Fatalf("loadDetailCmd err = %v", msg.err)
	}
	if !strings.Contains(msg.text, "TKT-001") {
		t.Fatalf("detail missing ID: %q", msg.text)
	}
}

func TestHelpers(t *testing.T) {
	if got := truncate("abcdef", 4); got != "abc…" {
		t.Fatalf("truncate = %q, want %q", got, "abc…")
	}
	if got := dropLastRune("a😊"); got != "a" {
		t.Fatalf("dropLastRune = %q, want %q", got, "a")
	}
}

func TestViewVariants(t *testing.T) {
	now := time.Now().UTC()
	b := newFakeBackend(
		&ticket.Ticket{ID: "TKT-001", State: ticket.State("todo"), Priority: 1, Title: "One", CreatedAt: now, UpdatedAt: now, CreatedBy: "human"},
	)

	m := newBoardModelWithDefaultCfg(b, "human:test")
	m.allTickets = []*ticket.Ticket{{ID: "TKT-001", State: ticket.State("todo"), Priority: 1, Title: "One", CreatedAt: now}}
	m.rebuildColumns("")
	m.width = 120

	board := m.View()
	if !strings.Contains(board, "TO DO") {
		t.Fatalf("board view missing TO DO column:\n%s", board)
	}

	m.detailOpen = true
	m.detailText = "detail body"
	detail := m.View()
	if !strings.Contains(detail, "detail body") {
		t.Fatalf("detail view missing body:\n%s", detail)
	}

	m.detailText = ""
	emptyDetail := m.viewDetail()
	if !strings.Contains(emptyDetail, "No detail available") {
		t.Fatalf("expected fallback detail text, got:\n%s", emptyDetail)
	}
}
