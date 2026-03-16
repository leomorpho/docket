package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/leomorpho/docket/internal/lifecycle"
)

const (
	lifecyclePhaseStartWorkflow = "start.workflow"
	lifecyclePhaseACCheck       = "ac.check"
)

func lifecycleStart(out io.Writer, command, ticketID, actor string) *lifecycle.Recorder {
	rec, err := lifecycle.StartRun(lifecycle.RunInput{
		RepoRoot: repo,
		Command:  command,
		TicketID: strings.TrimSpace(ticketID),
		Actor:    strings.TrimSpace(actor),
	})
	if err != nil {
		fmt.Fprintf(out, "docket: warning: lifecycle run.start emit failed: %v\n", err)
		return nil
	}
	return rec
}

func lifecyclePhaseEnd(out io.Writer, rec *lifecycle.Recorder, phase, status string) {
	if rec == nil {
		return
	}
	if err := rec.PhaseEnd(phase, status); err != nil {
		fmt.Fprintf(out, "docket: warning: lifecycle phase.end emit failed: %v\n", err)
	}
}

func lifecycleToolFailure(out io.Writer, rec *lifecycle.Recorder, phase, tool string, runErr error) {
	if rec == nil {
		return
	}
	if err := rec.ToolFailure(phase, tool, runErr); err != nil {
		fmt.Fprintf(out, "docket: warning: lifecycle tool.failure emit failed: %v\n", err)
	}
}

func lifecycleRunEnd(out io.Writer, rec *lifecycle.Recorder, status string) {
	if rec == nil {
		return
	}
	if err := rec.End(status); err != nil {
		fmt.Fprintf(out, "docket: warning: lifecycle run.end emit failed: %v\n", err)
	}
}
