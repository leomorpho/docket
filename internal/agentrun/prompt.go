package agentrun

import (
	"fmt"
	"strings"

	"github.com/leomorpho/docket/internal/ticket"
)

func DefaultImplementerPrompt(tkt *ticket.Ticket) string {
	ticketID := ""
	title := ""
	desc := ""
	if tkt != nil {
		ticketID = tkt.ID
		title = strings.TrimSpace(tkt.Title)
		desc = strings.TrimSpace(tkt.Description)
	}
	lines := []string{
		fmt.Sprintf("Work only ticket %s in this run.", ticketID),
		"Use test-driven development.",
		"Analyze requirements, write or update tests first, then implement the smallest passing change.",
	}
	if title != "" {
		lines = append(lines, fmt.Sprintf("Title: %s", title))
	}
	if desc != "" {
		lines = append(lines, fmt.Sprintf("Description: %s", desc))
	}
	if tkt != nil && len(tkt.AC) > 0 {
		lines = append(lines, "Acceptance Criteria:")
		for _, ac := range tkt.AC {
			lines = append(lines, "- "+ac.Description)
		}
	}
	lines = append(lines,
		"At the beginning, print exactly one plan line like: PLAN ticket="+ticketID+" steps=<N>.",
		"As you work, print step progress lines like: STEP ticket="+ticketID+" index=<I> status=in_progress title=\"<step title>\" and STEP ticket="+ticketID+" index=<I> status=done title=\"<step title>\".",
		"If useful, print status lines like: STATUS ticket="+ticketID+" phase=testing.",
		"Do not change ticket state yourself; the orchestrator handles state transitions and final validation.",
		"If a brief ticket comment materially helps future humans, you may add one, but do not spend time on ticket bookkeeping.",
		"Commit with a `Ticket: <TKT-NNN>` trailer after the code and tests are green.",
		fmt.Sprintf("Use `Ticket: %s` in your commit trailer for this ticket.", ticketID),
		"Finish by printing exactly one final line in one of these forms:",
		fmt.Sprintf("RESULT status=done ticket=%s role=implementer commit=<sha> tests=passed", ticketID),
		fmt.Sprintf("RESULT status=stuck ticket=%s role=implementer reason=\"<specific blocker>\"", ticketID),
		fmt.Sprintf("RESULT status=failed ticket=%s role=implementer reason=\"<failure reason>\"", ticketID),
	)
	return strings.Join(lines, "\n")
}

func DefaultReviewerPrompt(ticketID string) string {
	return fmt.Sprintf(
		"Review ticket %s only.\nAt the beginning, print exactly one plan line like: PLAN ticket=%s steps=<N>.\nCheck for bugs, regressions, acceptance-criteria gaps, and missing tests.\nReply with exactly one final line in this format:\nREVIEW status=approved ticket=%s role=reviewer\nor\nREVIEW status=changes_required ticket=%s role=reviewer required_changes=\"<specific required changes>\"",
		ticketID,
		ticketID,
		ticketID,
		ticketID,
	)
}

func DefaultFixPrompt(ticketID, requiredChanges string) string {
	return fmt.Sprintf(
		"Work only ticket %s in this fresh fix session.\nAt the beginning, print exactly one plan line like: PLAN ticket=%s steps=<N>.\nAddress these required review changes:\n%s\nUse test-driven development.\nCommit only if green.\nFinish by printing exactly one final line in one of these forms:\nRESULT status=done ticket=%s role=implementer commit=<sha> tests=passed\nRESULT status=stuck ticket=%s role=implementer reason=\"<specific blocker>\"\nRESULT status=failed ticket=%s role=implementer reason=\"<failure reason>\"",
		ticketID,
		ticketID,
		requiredChanges,
		ticketID,
		ticketID,
		ticketID,
	)
}
