package agentrun

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseResultLineRoundTripsDoneResult(t *testing.T) {
	t.Parallel()

	want := Result{
		Status:    StatusDone,
		TicketID:  "TKT-378",
		Role:      RoleImplementer,
		CommitSHA: "abc1234",
		Tests:     "passed",
	}

	line := want.Line()
	if !strings.HasPrefix(line, "RESULT ") {
		t.Fatalf("Line() = %q, want RESULT prefix", line)
	}

	got, err := ParseResultLine(line)
	if err != nil {
		t.Fatalf("ParseResultLine() error = %v", err)
	}
	if got != want {
		t.Fatalf("ParseResultLine() = %#v, want %#v", got, want)
	}
}

func TestParseResultLineRoundTripsStuckAndFailedResults(t *testing.T) {
	t.Parallel()

	cases := []Result{
		{
			Status:   StatusStuck,
			TicketID: "TKT-381",
			Role:     RoleImplementer,
			Reason:   "baseline tests failing",
		},
		{
			Status:   StatusFailed,
			TicketID: "TKT-381",
			Role:     RoleImplementer,
			Reason:   "codex process exited unexpectedly",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.Status), func(t *testing.T) {
			t.Parallel()
			got, err := ParseResultLine(tc.Line())
			if err != nil {
				t.Fatalf("ParseResultLine() error = %v", err)
			}
			if got != tc {
				t.Fatalf("ParseResultLine() = %#v, want %#v", got, tc)
			}
		})
	}
}

func TestParseResultLineRejectsMalformedOrIncompleteLines(t *testing.T) {
	t.Parallel()

	cases := []string{
		"",
		"done ticket=TKT-1",
		"RESULT ticket=TKT-1",
		"RESULT status=done",
		"RESULT status=done ticket=TKT-1 role=implementer",
		"RESULT status=stuck ticket=TKT-1 role=implementer",
		"RESULT status=done ticket=TKT-1 role=implementer reason=unexpected",
		"RESULT status=reviewing ticket=TKT-1 role=implementer",
	}

	for _, input := range cases {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseResultLine(input); err == nil {
				t.Fatalf("ParseResultLine(%q) unexpectedly succeeded", input)
			}
		})
	}
}

func TestParseReviewLineRoundTripsApprovedAndChangesRequired(t *testing.T) {
	t.Parallel()

	cases := []ReviewResult{
		{
			Status:   ReviewApproved,
			TicketID: "TKT-375",
			Role:     RoleReviewer,
		},
		{
			Status:          ReviewChangesRequired,
			TicketID:        "TKT-375",
			Role:            RoleReviewer,
			RequiredChanges: "missing regression test coverage",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.Status), func(t *testing.T) {
			t.Parallel()
			got, err := ParseReviewLine(tc.Line())
			if err != nil {
				t.Fatalf("ParseReviewLine() error = %v", err)
			}
			if got != tc {
				t.Fatalf("ParseReviewLine() = %#v, want %#v", got, tc)
			}
		})
	}
}

func TestParseResultAndReviewLinesHandleQuotedValuesAndRejectBrokenQuotes(t *testing.T) {
	t.Parallel()

	t.Run("result with quoted reason", func(t *testing.T) {
		t.Parallel()
		line := `RESULT status=stuck ticket=TKT-381 role=implementer reason="baseline tests failed after \"seed\" setup"`
		got, err := ParseResultLine(line)
		if err != nil {
			t.Fatalf("ParseResultLine() error = %v", err)
		}
		if got.Reason != `baseline tests failed after "seed" setup` {
			t.Fatalf("unexpected reason: %#v", got)
		}
	})

	t.Run("review with quoted required changes", func(t *testing.T) {
		t.Parallel()
		line := `REVIEW status=changes_required ticket=TKT-375 role=reviewer required_changes="add \"resume\" regression coverage"`
		got, err := ParseReviewLine(line)
		if err != nil {
			t.Fatalf("ParseReviewLine() error = %v", err)
		}
		if got.RequiredChanges != `add "resume" regression coverage` {
			t.Fatalf("unexpected review: %#v", got)
		}
	})

	t.Run("reject unterminated quoted field", func(t *testing.T) {
		t.Parallel()
		badLines := []string{
			`RESULT status=stuck ticket=TKT-381 role=implementer reason="unterminated`,
			`REVIEW status=changes_required ticket=TKT-375 role=reviewer required_changes="unterminated`,
		}
		for _, line := range badLines {
			line := line
			t.Run(line, func(t *testing.T) {
				t.Parallel()
				if strings.HasPrefix(line, "RESULT ") {
					if _, err := ParseResultLine(line); err == nil {
						t.Fatalf("ParseResultLine(%q) unexpectedly succeeded", line)
					}
					return
				}
				if _, err := ParseReviewLine(line); err == nil {
					t.Fatalf("ParseReviewLine(%q) unexpectedly succeeded", line)
				}
			})
		}
	})
}

func TestRunSpecAndRunRecordValidateContractFields(t *testing.T) {
	t.Parallel()

	spec := RunSpec{
		TicketID:     "TKT-378",
		Role:         RoleImplementer,
		RepoRoot:     "/repo",
		WorktreePath: "/repo/.worktrees/TKT-378",
		Branch:       "docket/TKT-378",
		Prompt:       "Work only ticket TKT-378 in this run.",
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("RunSpec.Validate() error = %v", err)
	}

	record := RunRecord{
		TicketID:     spec.TicketID,
		Role:         spec.Role,
		Adapter:      "codex",
		RepoRoot:     spec.RepoRoot,
		WorktreePath: spec.WorktreePath,
		Branch:       spec.Branch,
		SessionID:    "session-123",
		StartedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("RunRecord.Validate() error = %v", err)
	}
}

func TestStatusIsTerminal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		status Status
		want   bool
	}{
		{status: StatusRunning, want: false},
		{status: StatusDone, want: true},
		{status: StatusStuck, want: true},
		{status: StatusFailed, want: true},
		{status: Status("unknown"), want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.status), func(t *testing.T) {
			t.Parallel()
			if got := tc.status.IsTerminal(); got != tc.want {
				t.Fatalf("Status(%q).IsTerminal() = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

func TestValidationInputJSONShapeIsStable(t *testing.T) {
	t.Parallel()

	input := ValidationInput{
		TicketID:     "TKT-378",
		RepoRoot:     "/repo",
		WorktreePath: "/repo/.worktrees/TKT-378",
		Branch:       "docket/TKT-378",
		Result: Result{
			Status:    StatusDone,
			TicketID:  "TKT-378",
			Role:      RoleImplementer,
			CommitSHA: "abc123",
			Tests:     "passed",
		},
	}

	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal ValidationInput: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal ValidationInput: %v", err)
	}
	want := map[string]any{
		"ticket_id":     "TKT-378",
		"repo_root":     "/repo",
		"worktree_path": "/repo/.worktrees/TKT-378",
		"branch":        "docket/TKT-378",
		"result": map[string]any{
			"status":    "done",
			"ticket_id": "TKT-378",
			"role":      "implementer",
			"commit":    "abc123",
			"tests":     "passed",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ValidationInput JSON mismatch\n got: %#v\nwant: %#v", got, want)
	}
}
