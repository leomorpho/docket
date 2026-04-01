package check

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/ticket"
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
	b := newFake(&ticket.Ticket{ID: "TKT-001", State: ticket.State("running"), UpdatedAt: now.Add(-8 * 24 * time.Hour)})
	b.validate["TKT-001"] = []store.ValidationError{{Field: "state", Message: "bad"}}

	c := NewChecker(b, ticket.DefaultConfig())
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
	blocker := &ticket.Ticket{ID: "TKT-002", State: ticket.State("validated"), UpdatedAt: now}
	target := &ticket.Ticket{ID: "TKT-001", State: ticket.State("running"), BlockedBy: []string{"TKT-002"}, UpdatedAt: now.Add(-8 * 24 * time.Hour)}
	b := newFake(blocker, target)

	c := NewChecker(b, ticket.DefaultConfig())
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

func TestChecker_R006FixHonorsConfigForReviewState(t *testing.T) {
	now := time.Now().UTC()
	cfg := ticket.DefaultConfig()
	review := cfg.States["validated"]
	review.BlocksDependents = false
	cfg.States["validated"] = review

	blocker := &ticket.Ticket{ID: "TKT-002", State: ticket.State("validated"), UpdatedAt: now}
	target := &ticket.Ticket{ID: "TKT-001", State: ticket.State("running"), BlockedBy: []string{"TKT-002"}, UpdatedAt: now}
	b := newFake(blocker, target)

	c := NewChecker(b, cfg)
	if _, err := c.Run(context.Background(), []*ticket.Ticket{target}, true); err != nil {
		t.Fatalf("run with config-aware fix failed: %v", err)
	}

	updated, _ := b.GetTicket(context.Background(), "TKT-001")
	if len(updated.BlockedBy) != 0 {
		t.Fatalf("expected validated blocker removed when config allows it, got %v", updated.BlockedBy)
	}
}

func TestChecker_R001UsesConfiguredActiveRole(t *testing.T) {
	now := time.Now().UTC()
	cfg := &ticket.Config{
		States: map[string]ticket.StateConfig{
			"queued":  {Label: "Queued", Open: true, Column: 0, Next: []string{"coding"}, Roles: []string{"intake"}, Startable: true},
			"coding":  {Label: "Coding", Open: true, Column: 1, Next: []string{"testing"}, Roles: []string{"active"}},
			"testing": {Label: "Testing", Open: true, Column: 2, Next: []string{"qa"}, Roles: []string{"active"}},
			"qa":      {Label: "QA", Open: true, Column: 3, Next: []string{"shipped"}, Roles: []string{"review"}},
			"shipped": {Label: "Shipped", Open: false, Column: 4, Next: []string{}, Roles: []string{"completed"}, Terminal: true},
		},
	}
	b := newFake(&ticket.Ticket{ID: "TKT-900", State: ticket.State("testing"), UpdatedAt: now.Add(-8 * 24 * time.Hour)})
	c := NewChecker(b, cfg)
	c.Now = func() time.Time { return now }

	findings, err := c.Run(context.Background(), []*ticket.Ticket{b.tickets["TKT-900"]}, false)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(findings) == 0 || findings[0].Rule != "R001" {
		t.Fatalf("expected R001 finding for configured active-role state, got %+v", findings)
	}
}

func TestChecker_ReportsDescendantClosureIssues(t *testing.T) {
	now := time.Now().UTC()
	parent := &ticket.Ticket{ID: "TKT-172", State: ticket.State("validated"), UpdatedAt: now}
	childOpen := &ticket.Ticket{ID: "TKT-174", State: ticket.State("running"), Parent: "TKT-172", UpdatedAt: now}
	childInvalid := &ticket.Ticket{ID: "TKT-175", State: ticket.State("validated"), Parent: "TKT-172", UpdatedAt: now}
	childBlocked := &ticket.Ticket{ID: "TKT-176", State: ticket.State("validated"), Parent: "TKT-172", BlockedBy: []string{"TKT-177"}, UpdatedAt: now}
	blocker := &ticket.Ticket{ID: "TKT-177", State: ticket.State("running"), UpdatedAt: now}

	b := newFake(parent, childOpen, childInvalid, childBlocked, blocker)
	b.validate["TKT-175"] = []store.ValidationError{{Field: "handoff", Message: "missing required subsection: AC status"}}

	c := NewChecker(b, ticket.DefaultConfig())
	c.Now = func() time.Time { return now }

	findings, err := c.Run(context.Background(), []*ticket.Ticket{parent}, false)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	var messages []string
	for _, finding := range findings {
		if finding.Rule == "R009" {
			messages = append(messages, finding.Message)
		}
	}
	if len(messages) != 3 {
		t.Fatalf("expected 3 descendant-governance findings, got %+v", findings)
	}
	joined := strings.Join(messages, "\n")
	for _, want := range []string{
		"descendant TKT-174 is still running",
		"descendant TKT-175 failed validation: handoff: missing required subsection: AC status",
		"descendant TKT-176 is still blocked by TKT-177 (running)",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected descendant finding %q in %q", want, joined)
		}
	}
}
