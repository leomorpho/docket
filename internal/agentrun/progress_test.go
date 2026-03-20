package agentrun

import "testing"

func TestParsePlanStepAndStatusLines(t *testing.T) {
	t.Parallel()

	plan, err := ParsePlanLine("PLAN ticket=TKT-390 steps=8")
	if err != nil || plan.Steps != 8 || plan.TicketID != "TKT-390" {
		t.Fatalf("ParsePlanLine() = %#v err=%v", plan, err)
	}

	step, err := ParseStepLine(`STEP ticket=TKT-390 index=2 status=in_progress title="write failing test"`)
	if err != nil || step.Index != 2 || step.Status != StepInProgress || step.Title != "write failing test" {
		t.Fatalf("ParseStepLine() = %#v err=%v", step, err)
	}

	status, err := ParseStatusLine("STATUS ticket=TKT-390 phase=testing")
	if err != nil || status.Phase != "testing" || status.TicketID != "TKT-390" {
		t.Fatalf("ParseStatusLine() = %#v err=%v", status, err)
	}
}

func TestParsePlanStepAndStatusLinesRejectMalformedInput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		line  string
		parse func(string) error
	}{
		{
			name: "plan missing steps",
			line: "PLAN ticket=TKT-390",
			parse: func(line string) error {
				_, err := ParsePlanLine(line)
				return err
			},
		},
		{
			name: "plan zero steps",
			line: "PLAN ticket=TKT-390 steps=0",
			parse: func(line string) error {
				_, err := ParsePlanLine(line)
				return err
			},
		},
		{
			name: "step unsupported status",
			line: `STEP ticket=TKT-390 index=1 status=queued title="bad"`,
			parse: func(line string) error {
				_, err := ParseStepLine(line)
				return err
			},
		},
		{
			name: "step missing title",
			line: "STEP ticket=TKT-390 index=1 status=done",
			parse: func(line string) error {
				_, err := ParseStepLine(line)
				return err
			},
		},
		{
			name: "status missing phase",
			line: "STATUS ticket=TKT-390",
			parse: func(line string) error {
				_, err := ParseStatusLine(line)
				return err
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.parse(tc.line); err == nil {
				t.Fatalf("expected parse error for %q", tc.line)
			}
		})
	}
}
