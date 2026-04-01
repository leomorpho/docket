package ticket

import "testing"

func TestWorkflowEvaluatorDefaultConfigSemantics(t *testing.T) {
	ev := NewWorkflowEvaluator(DefaultConfig())

	if !ev.CanTransition("ready", "running") {
		t.Fatal("expected ready -> running transition to be allowed")
	}
	if ev.CanTransition("ready", "validated") {
		t.Fatal("expected ready -> validated transition to be disallowed")
	}
	if !ev.StateHasRole("running", "active") {
		t.Fatal("expected running to have active role")
	}
	if ev.IsStartable("draft") || !ev.IsStartable("ready") {
		t.Fatal("expected only ready to be startable in the default config")
	}
	if ev.IsReviewable("validated") {
		t.Fatal("expected validated to not be reviewable")
	}
	if ev.BlocksDependents("validated") {
		t.Fatal("expected validated to be non-blocking in default config")
	}
	if !ev.DependencyBlocks(&Ticket{State: "running"}) {
		t.Fatal("expected active dependency to block")
	}
	if ev.DependencyBlocks(&Ticket{State: "validated"}) {
		t.Fatal("expected validated dependency to be non-blocking in default config")
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
