package hooks

import (
	"errors"
	"testing"
)

func TestHookFiringAtBoundary(t *testing.T) {
	m := NewManager()
	reviewHits := 0
	m.Register(Registration{
		Name:  "review-advisory",
		Event: EventReviewGate,
		Mode:  ModeAdvisory,
		Run: func(Context) error {
			reviewHits++
			return errors.New("advisory")
		},
	})

	if _, err := m.Run(EventRunStart, Context{}); err != nil {
		t.Fatalf("unexpected run-start error: %v", err)
	}
	if reviewHits != 0 {
		t.Fatalf("expected no review advisory hit on start boundary")
	}

	advisory, err := m.Run(EventReviewGate, Context{})
	if err != nil {
		t.Fatalf("unexpected review advisory error: %v", err)
	}
	if reviewHits != 1 {
		t.Fatalf("expected one review advisory hit, got %d", reviewHits)
	}
	if len(advisory) != 1 {
		t.Fatalf("expected one advisory message, got %d", len(advisory))
	}
}

func TestEnforcementHookBlocksAtBoundary(t *testing.T) {
	m := NewManager()
	RegisterCoreHooks(m)

	_, err := m.Run(EventPrivileged, Context{
		TicketID:             "TKT-199",
		TargetState:          "done",
		PrivilegedAuthorized: false,
	})
	if err == nil {
		t.Fatalf("expected privileged enforcement failure")
	}
}
