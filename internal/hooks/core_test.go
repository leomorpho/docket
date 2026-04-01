package hooks

import "testing"

func TestRegisterCoreHooksOmitsDefaultReviewAndQAEvents(t *testing.T) {
	m := NewManager()
	RegisterCoreHooks(m)

	if len(m.byEvent[EventRunStart]) == 0 {
		t.Fatal("expected default core hooks to keep a run.start registration")
	}
	if regs := m.byEvent[EventReviewGate]; len(regs) > 0 {
		t.Fatalf("default core hooks should not register review gate handlers, got %d", len(regs))
	}
	if regs := m.byEvent[EventQAGate]; len(regs) > 0 {
		t.Fatalf("default core hooks should not register QA gate handlers, got %d", len(regs))
	}
}
