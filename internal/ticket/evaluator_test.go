package ticket

import "testing"

func TestWorkflowEvaluatorDefaultConfigSemantics(t *testing.T) {
	ev := NewWorkflowEvaluator(DefaultConfig())

	if !ev.CanTransition("todo", "in-progress") {
		t.Fatal("expected todo -> in-progress transition to be allowed")
	}
	if ev.CanTransition("todo", "done") {
		t.Fatal("expected todo -> done transition to be disallowed")
	}
	if !ev.StateHasRole("in-progress", "active") {
		t.Fatal("expected in-progress to have active role")
	}
	if !ev.IsStartable("backlog") || !ev.IsStartable("todo") {
		t.Fatal("expected backlog/todo to be startable")
	}
	if !ev.IsReviewable("in-review") {
		t.Fatal("expected in-review to be reviewable")
	}
	if ev.BlocksDependents("in-review") {
		t.Fatal("expected in-review to be non-blocking in default config")
	}
	if !ev.DependencyBlocks(&Ticket{State: "in-progress"}) {
		t.Fatal("expected active dependency to block")
	}
	if ev.DependencyBlocks(&Ticket{State: "in-review"}) {
		t.Fatal("expected review dependency to be non-blocking in default config")
	}
	if !ev.DependencyBlocks(nil) {
		t.Fatal("expected missing dependency to be treated as blocking")
	}
}

func TestWorkflowEvaluatorCustomConfigSemantics(t *testing.T) {
	cfg := &Config{
		States: map[string]StateConfig{
			"queued": {
				Label:            "Queued",
				Open:             true,
				Column:           0,
				Next:             []string{"building"},
				Roles:            []string{"intake"},
				Startable:        true,
				BlocksDependents: true,
			},
			"building": {
				Label:            "Building",
				Open:             true,
				Column:           1,
				Next:             []string{"qa"},
				Roles:            []string{"active"},
				BlocksDependents: true,
			},
			"qa": {
				Label:            "QA",
				Open:             true,
				Column:           2,
				Next:             []string{"shipped"},
				Roles:            []string{"review"},
				Reviewable:       true,
				BlocksDependents: true,
			},
			"shipped": {
				Label:    "Shipped",
				Open:     false,
				Column:   3,
				Next:     []string{},
				Roles:    []string{"completed"},
				Terminal: true,
			},
		},
		DefaultState: "queued",
	}
	ev := NewWorkflowEvaluator(cfg)

	if !ev.CanTransition("building", "qa") {
		t.Fatal("expected building -> qa transition to be allowed")
	}
	if ev.CanTransition("queued", "qa") {
		t.Fatal("expected queued -> qa transition to be disallowed")
	}
	if !ev.StateHasRole("qa", "review") {
		t.Fatal("expected qa to have review role")
	}
	if !ev.IsStartable("queued") {
		t.Fatal("expected queued to be startable")
	}
	if !ev.IsReviewable("qa") {
		t.Fatal("expected qa to be reviewable")
	}
	if !ev.BlocksDependents("qa") {
		t.Fatal("expected qa to block dependents in custom config")
	}
	if !ev.DependencyBlocks(&Ticket{State: "qa"}) {
		t.Fatal("expected qa dependency to block in custom config")
	}
}

