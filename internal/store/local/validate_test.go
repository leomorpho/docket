package local

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/ticket"
)

func TestValidateFile(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()

	// 1. Valid ticket
	now := time.Now().UTC().Truncate(time.Second)
	t1 := &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "Valid Ticket",
		State:       ticket.State("ready"),
		Priority:    1,
		Labels:      []string{"bug"},
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:tester",
		Description: "Likely paths: internal/store/local/validate.go and internal/store/local/ready_contract.go. Verify commands: go test ./internal/store/local -run TestValidateFile -count=1. This ticket has enough execution context to pass the quality check so that agents can execute it without asking clarifying questions. Out of scope: unrelated workflow cleanup.",
		AC: []ticket.AcceptanceCriterion{
			{Description: "Has AC item one", Done: false, Run: "test -f README.md"},
			{Description: "Has AC item two", Done: false, VerificationSteps: []string{"Inspect validation output"}},
		},
		Handoff: "Has handoff",
	}
	s.CreateTicket(ctx, t1)

	errs, warns, err := s.ValidateFile(t1.ID)
	if err != nil {
		t.Fatalf("ValidateFile failed: %v", err)
	}
	if len(errs) > 0 {
		t.Errorf("expected no errors for valid ticket, got: %v", errs)
	}
	if len(warns) > 0 {
		t.Errorf("expected no warnings for valid ticket, got: %v", warns)
	}

	// 2. Invalid ticket (wrong state, missing description)
	t2 := &ticket.Ticket{
		ID:        "TKT-002",
		Seq:       2,
		Title:     "Invalid Ticket",
		State:     "blocked", // invalid state
		Priority:  1,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "human:tester",
		// No description, no AC
	}
	s.CreateTicket(ctx, t2)
	errs, _, _ = s.ValidateFile(t2.ID)
	if len(errs) < 3 {
		t.Errorf("expected at least 3 errors (state, body desc, body ac), got: %d", len(errs))
	}

	// 3. ID mismatch
	// Manually write a file with mismatching ID in frontmatter
	path3 := filepath.Join(tmpDir, ".docket", "tickets", "TKT-003.md")
	t3 := *t1
	t3.ID = "TKT-999"
	content, _ := render(&t3)
	os.WriteFile(path3, []byte(content), 0644)
	errs, _, _ = s.ValidateFile("TKT-003")
	foundIDMismatch := false
	for _, e := range errs {
		if e.Field == "id" {
			foundIDMismatch = true
		}
	}
	if !foundIDMismatch {
		t.Errorf("expected ID mismatch error, but not found in %v", errs)
	}
}

func TestDetectCycles(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()

	now := time.Now().UTC()
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-001", BlockedBy: []string{"TKT-002"}, Title: "T1", State: "draft", CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{}}})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-002", BlockedBy: []string{"TKT-003"}, Title: "T2", State: "draft", CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{}}})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-003", BlockedBy: []string{"TKT-001"}, Title: "T3", State: "draft", CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{}}})

	err := s.detectCycles()
	if err == nil {
		t.Fatal("expected cycle detection error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle detected") {
		t.Errorf("expected cycle detected message, got: %v", err)
	}
}

func TestValidateFile_RequiresStructuredHandoffForReviewAndDone(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	t1 := &ticket.Ticket{
		ID:          "TKT-010",
		Seq:         10,
		Title:       "Needs handoff",
		State:       ticket.State("validated"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:tester",
		Description: "desc",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		Handoff:     "partial handoff",
	}
	if err := s.CreateTicket(ctx, t1); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	errs, _, err := s.ValidateFile("TKT-010")
	if err != nil {
		t.Fatalf("ValidateFile failed: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected handoff structure validation errors")
	}

	t1.Handoff = "*Last updated: 2026-03-09T15:00:00Z by agent:test*\n\n**Current state:** validated.\n\n**Decisions made:** decision.\n\n**Files touched:** file.\n\n**Remaining work:** none.\n\n**AC status:** complete."
	if err := s.UpdateTicket(ctx, t1); err != nil {
		t.Fatalf("UpdateTicket failed: %v", err)
	}
	errs, _, err = s.ValidateFile("TKT-010")
	if err != nil {
		t.Fatalf("ValidateFile failed: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("expected no handoff structure errors, got %v", errs)
	}
}

func TestValidateFile_HandoffSectionsConfigDriven(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	cfg := ticket.DefaultConfig()
	cfg.HandoffSections = []string{"Current state", "Risks"}
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	t1 := &ticket.Ticket{
		ID:          "TKT-011",
		Seq:         11,
		Title:       "Config handoff sections",
		State:       ticket.State("validated"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:tester",
		Description: "desc",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		Handoff:     "**Current state:** good",
	}
	if err := s.CreateTicket(ctx, t1); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	errs, _, err := s.ValidateFile("TKT-011")
	if err != nil {
		t.Fatalf("ValidateFile failed: %v", err)
	}
	missingRisks := false
	for _, e := range errs {
		if e.Field == "handoff" && strings.Contains(strings.ToLower(e.Message), "risks") {
			missingRisks = true
		}
	}
	if !missingRisks {
		t.Fatalf("expected missing Risks section error, got %v", errs)
	}

	cfg.HandoffSections = []string{"Current state"}
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	errs, _, err = s.ValidateFile("TKT-011")
	if err != nil {
		t.Fatalf("ValidateFile failed: %v", err)
	}
	for _, e := range errs {
		if e.Field == "handoff" && strings.Contains(strings.ToLower(e.Message), "risks") {
			t.Fatalf("did not expect Risks to be required after config update: %v", errs)
		}
	}
}

func TestValidateFile_RejectsNonLeafExecutionBlocker(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	parent := &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "Parent",
		State:       ticket.State("draft"),
		Priority:    1,
		Labels:      []string{"epic"},
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "Parent ticket with children",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
	}
	child := &ticket.Ticket{
		ID:          "TKT-002",
		Seq:         2,
		Title:       "Child",
		Parent:      "TKT-001",
		State:       ticket.State("draft"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "Child ticket",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
	}
	blocked := &ticket.Ticket{
		ID:          "TKT-003",
		Seq:         3,
		Title:       "Blocked leaf",
		State:       ticket.State("draft"),
		Priority:    1,
		BlockedBy:   []string{"TKT-001"},
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "Blocked ticket",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
	}

	for _, tk := range []*ticket.Ticket{parent, child, blocked} {
		if err := s.CreateTicket(ctx, tk); err != nil {
			t.Fatalf("create %s: %v", tk.ID, err)
		}
	}

	errs, _, err := s.ValidateFile("TKT-003")
	if err != nil {
		t.Fatalf("ValidateFile failed: %v", err)
	}
	found := false
	for _, item := range errs {
		if item.Field == "blocked_by[0]" && strings.Contains(item.Message, "must be a leaf ticket") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected non-leaf blocker validation error, got %v", errs)
	}
}

func TestValidateFile_UsesWorkflowRolesForHandoffAndComments(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	cfg := &ticket.Config{
		Backend: "local",
		States: map[string]ticket.StateConfig{
			"queued": {
				Label:            "Queued",
				Open:             true,
				Column:           0,
				Next:             []string{"coding"},
				Roles:            []string{"intake"},
				Startable:        true,
				BlocksDependents: true,
			},
			"coding": {
				Label:            "Coding",
				Open:             true,
				Column:           1,
				Next:             []string{"testing"},
				Roles:            []string{"active"},
				BlocksDependents: true,
			},
			"testing": {
				Label:            "Testing",
				Open:             true,
				Column:           2,
				Next:             []string{"qa"},
				Roles:            []string{"active"},
				BlocksDependents: true,
			},
			"qa": {
				Label:            "QA",
				Open:             true,
				Column:           3,
				Next:             []string{"shipped"},
				Roles:            []string{"review"},
				Reviewable:       true,
				BlocksDependents: true,
			},
			"shipped": {
				Label:    "Shipped",
				Open:     false,
				Column:   4,
				Next:     []string{},
				Roles:    []string{"completed"},
				Terminal: true,
			},
		},
		DefaultState:    "queued",
		DefaultPriority: 10,
		HandoffSections: []string{"Current state", "Decisions made"},
	}
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	active := &ticket.Ticket{
		ID:          "TKT-020",
		Seq:         20,
		Title:       "Active warning",
		State:       "testing",
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "Likely paths: internal/store/local/validate.go and internal/store/local/ready_contract.go. Verify commands: go test ./internal/store/local -run TestValidateFile_UsesWorkflowRolesForHandoffAndComments -count=1. This description is long enough to avoid the short description warning in validation for this ticket, and it includes enough execution context for runnable-state validation. Out of scope: unrelated backlog migration or ticket taxonomy cleanup.",
		AC: []ticket.AcceptanceCriterion{
			{Description: "A1", Run: "printf ok >/dev/null"},
			{Description: "A2", VerificationSteps: []string{"Inspect active warning behavior"}},
		},
	}
	if err := s.CreateTicket(ctx, active); err != nil {
		t.Fatalf("CreateTicket active failed: %v", err)
	}

	errs, warns, err := s.ValidateFile("TKT-020")
	if err != nil {
		t.Fatalf("ValidateFile active failed: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected no active-state errors, got %v", errs)
	}
	foundCommentWarn := false
	for _, warn := range warns {
		if warn.Field == "quality.comments" {
			foundCommentWarn = true
			break
		}
	}
	if !foundCommentWarn {
		t.Fatalf("expected active-role comment warning, got %v", warns)
	}

	review := &ticket.Ticket{
		ID:          "TKT-021",
		Seq:         21,
		Title:       "Review handoff",
		State:       "qa",
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "This description is long enough to avoid unrelated quality warnings during review-state validation coverage.",
		AC: []ticket.AcceptanceCriterion{
			{Description: "A1"},
			{Description: "A2"},
		},
		Handoff: "**Current state:** good",
	}
	if err := s.CreateTicket(ctx, review); err != nil {
		t.Fatalf("CreateTicket review failed: %v", err)
	}

	errs, _, err = s.ValidateFile("TKT-021")
	if err != nil {
		t.Fatalf("ValidateFile review failed: %v", err)
	}
	missingDecisions := false
	for _, e := range errs {
		if e.Field == "handoff" && strings.Contains(strings.ToLower(e.Message), "decisions made") {
			missingDecisions = true
			break
		}
	}
	if !missingDecisions {
		t.Fatalf("expected review-role handoff error, got %v", errs)
	}

	completed := &ticket.Ticket{
		ID:          "TKT-022",
		Seq:         22,
		Title:       "Completed handoff",
		State:       "shipped",
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "This description is long enough to avoid unrelated quality warnings during completed-state validation coverage.",
		AC: []ticket.AcceptanceCriterion{
			{Description: "A1"},
			{Description: "A2"},
		},
	}
	if err := s.CreateTicket(ctx, completed); err != nil {
		t.Fatalf("CreateTicket completed failed: %v", err)
	}

	errs, _, err = s.ValidateFile("TKT-022")
	if err != nil {
		t.Fatalf("ValidateFile completed failed: %v", err)
	}
	foundHandoffRequired := false
	for _, e := range errs {
		if e.Field == "handoff" && strings.Contains(strings.ToLower(e.Message), "required") {
			foundHandoffRequired = true
			break
		}
	}
	if !foundHandoffRequired {
		t.Fatalf("expected completed-role handoff requirement, got %v", errs)
	}
}

func TestValidate(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// Valid ticket
	t1 := &ticket.Ticket{
		ID: "TKT-001", Seq: 1, Title: "T1", State: "draft", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me",
		Description: "This is a long description so it passes validation quality checks easily and without any issues.",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
	}
	s.CreateTicket(ctx, t1)

	// 1. Validate specific ticket
	errs, err := s.Validate(ctx, "TKT-001")
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if len(errs) > 0 {
		t.Errorf("expected no errors, got %v", errs)
	}

	// 2. Validate all (alias for ValidateAll)
	errs, err = s.Validate(ctx, "")
	if err != nil {
		t.Fatalf("Validate all failed: %v", err)
	}
}

func TestValidateAll_SkipsNonTicketMarkdownFiles(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)

	ticketsDir := filepath.Join(tmpDir, ".docket", "tickets")
	if err := os.MkdirAll(ticketsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ticketsDir, "README.md"), []byte("# Not a ticket\n"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	allErrs, _, err := s.ValidateAll(context.Background())
	if err != nil {
		t.Fatalf("ValidateAll failed: %v", err)
	}
	if _, ok := allErrs["README"]; ok {
		t.Fatalf("expected README.md to be skipped, got errors: %v", allErrs["README"])
	}
}

func TestValidateAll(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "T1", State: "draft", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{}}})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-002", Seq: 2, Title: "T2", State: "draft", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{}}})

	allErrs, allWarns, err := s.ValidateAll(ctx)
	if err != nil {
		t.Fatalf("ValidateAll failed: %v", err)
	}
	if len(allErrs) != 2 {
		t.Errorf("expected errors for 2 tickets, got %d", len(allErrs))
	}
	if len(allWarns) != 2 {
		t.Errorf("expected warnings for 2 tickets, got %d", len(allWarns))
	}
}

func TestValidateFile_ReadyContractRejectsCoordinationTicketInReady(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	tk := &ticket.Ticket{
		ID:          "TKT-100",
		Seq:         100,
		Title:       "[Epic] Coordination only",
		State:       ticket.State("ready"),
		Priority:    1,
		Labels:      []string{"epic"},
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "Likely paths: docs/north-star.md. This structural ticket should never become runnable. Out of scope: implementation details.",
		AC: []ticket.AcceptanceCriterion{
			{Description: "A1", Run: "printf ok >/dev/null"},
			{Description: "A2", VerificationSteps: []string{"Inspect queue status"}},
		},
	}
	if err := s.CreateTicket(ctx, tk); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	errs, _, err := s.ValidateFile("TKT-100")
	if err != nil {
		t.Fatalf("ValidateFile failed: %v", err)
	}
	found := false
	for _, issue := range errs {
		if issue.Field == "state" && strings.Contains(issue.Message, "coordination tickets") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected ready-contract coordination error, got %v", errs)
	}
}

func TestValidateFile_ReadyContractRequiresVerificationAndSections(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	tk := &ticket.Ticket{
		ID:          "TKT-101",
		Seq:         101,
		Title:       "Runnable but underspecified",
		State:       ticket.State("ready"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "This ticket lacks the sections required for runnable work even though it is marked ready for execution by the runtime.",
		AC: []ticket.AcceptanceCriterion{
			{Description: "A1"},
			{Description: "A2"},
		},
	}
	if err := s.CreateTicket(ctx, tk); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	errs, _, err := s.ValidateFile("TKT-101")
	if err != nil {
		t.Fatalf("ValidateFile failed: %v", err)
	}
	joined := []string{}
	for _, issue := range errs {
		joined = append(joined, issue.Message)
	}
	msg := strings.Join(joined, "\n")
	for _, want := range []string{"Likely paths", "Out of scope", "run` or `verification_steps"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected ready-contract error containing %q, got %s", want, msg)
		}
	}
}
