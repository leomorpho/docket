package check

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/leoaudibert/docket/internal/store"
	"github.com/leoaudibert/docket/internal/ticket"
)

type fakeBackend struct {
	tickets  map[string]*ticket.Ticket
	validate map[string][]store.ValidationError
}

func newFake(ts ...*ticket.Ticket) *fakeBackend {
	m := map[string]*ticket.Ticket{}
	for _, t := range ts {
		cp := *t
		cp.BlockedBy = append([]string{}, t.BlockedBy...)
		m[t.ID] = &cp
	}
	return &fakeBackend{tickets: m, validate: map[string][]store.ValidationError{}}
}

func (f *fakeBackend) CreateTicket(ctx context.Context, t *ticket.Ticket) error {
	f.tickets[t.ID] = t
	return nil
}
func (f *fakeBackend) UpdateTicket(ctx context.Context, t *ticket.Ticket) error {
	cp := *t
	f.tickets[t.ID] = &cp
	return nil
}
func (f *fakeBackend) GetTicket(ctx context.Context, id string) (*ticket.Ticket, error) {
	if t, ok := f.tickets[id]; ok {
		cp := *t
		cp.BlockedBy = append([]string{}, t.BlockedBy...)
		return &cp, nil
	}
	return nil, nil
}
func (f *fakeBackend) GetRaw(ctx context.Context, id string) (string, error) { return "", nil }
func (f *fakeBackend) ListTickets(ctx context.Context, filter store.Filter) ([]*ticket.Ticket, error) {
	var out []*ticket.Ticket
	for _, t := range f.tickets {
		out = append(out, t)
	}
	return out, nil
}
func (f *fakeBackend) AddComment(ctx context.Context, id string, c ticket.Comment) error { return nil }
func (f *fakeBackend) LinkCommit(ctx context.Context, id string, sha string) error       { return nil }
func (f *fakeBackend) NextID(ctx context.Context) (string, int, error)                   { return "", 0, nil }
func (f *fakeBackend) Validate(ctx context.Context, id string) ([]store.ValidationError, error) {
	return f.validate[id], nil
}

func TestChecker_R001AndR008(t *testing.T) {
	now := time.Now().UTC()
	b := newFake(&ticket.Ticket{ID: "TKT-001", State: ticket.StateInProgress, UpdatedAt: now.Add(-8 * 24 * time.Hour)})
	b.validate["TKT-001"] = []store.ValidationError{{Field: "state", Message: "bad"}}

	c := NewChecker(b)
	c.Now = func() time.Time { return now }
	findings, err := c.Run(context.Background(), []*ticket.Ticket{b.tickets["TKT-001"]}, false)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(findings) < 2 {
		t.Fatalf("expected >=2 findings, got %d", len(findings))
	}
}

func TestChecker_R006Fix(t *testing.T) {
	now := time.Now().UTC()
	blocker := &ticket.Ticket{ID: "TKT-002", State: ticket.StateDone, UpdatedAt: now}
	target := &ticket.Ticket{ID: "TKT-001", State: ticket.StateInProgress, BlockedBy: []string{"TKT-002"}, UpdatedAt: now.Add(-8 * 24 * time.Hour)}
	b := newFake(blocker, target)

	c := NewChecker(b)
	c.Now = func() time.Time { return now }
	findings, err := c.Run(context.Background(), []*ticket.Ticket{target}, true)
	if err != nil {
		t.Fatalf("run with fix failed: %v", err)
	}

	updated, _ := b.GetTicket(context.Background(), "TKT-001")
	if len(updated.BlockedBy) != 0 {
		t.Fatalf("expected blocker removed, got %v", updated.BlockedBy)
	}
	if len(findings) == 0 || findings[0].Rule != "R001" || !strings.Contains(findings[0].Message, "No activity") {
		t.Fatalf("expected R001 finding after fix, got %+v", findings)
	}
}
