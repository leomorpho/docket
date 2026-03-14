package workflow

import (
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/ticket"
)

func TestUpdateStateCmdValidateRejectsInvalidTransition(t *testing.T) {
	cfg := ticket.DefaultConfig()
	tkt := &ticket.Ticket{
		ID:    "TKT-001",
		State: "todo",
	}
	cmd := UpdateStateCmd{To: "done"}
	if err := cmd.Validate(tkt, cfg); err == nil {
		t.Fatal("expected invalid transition error")
	}
}

func TestUpdateStateCmdApplySetsLifecycleFields(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	tkt := &ticket.Ticket{
		ID:    "TKT-002",
		State: "in-progress",
	}
	cmd := UpdateStateCmd{
		To:             "done",
		SetCompletedAt: true,
	}
	cmd.Apply(tkt, now)
	if tkt.State != "done" {
		t.Fatalf("expected state=done, got %s", tkt.State)
	}
	if tkt.UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt to be set")
	}
	if tkt.CompletedAt.IsZero() {
		t.Fatal("expected CompletedAt to be set")
	}
}
