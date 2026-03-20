package agentrun

import (
	"strings"
	"testing"

	"github.com/leomorpho/docket/internal/ticket"
)

func TestDefaultImplementerPromptKeepsBookkeepingOutOfBand(t *testing.T) {
	t.Parallel()

	prompt := DefaultImplementerPrompt(&ticket.Ticket{
		ID:          "TKT-390",
		Title:       "Prompt contract",
		Description: "Keep the implementer focused on code and tests.",
		AC: []ticket.AcceptanceCriterion{
			{Description: "go test passes"},
		},
	})

	for _, want := range []string{
		"Do not change ticket state yourself; the orchestrator handles state transitions and final validation.",
		"If a brief ticket comment materially helps future humans, you may add one, but do not spend time on ticket bookkeeping.",
		"Commit with a `Ticket: <TKT-NNN>` trailer after the code and tests are green.",
		"RESULT status=done ticket=TKT-390 role=implementer commit=<sha> tests=passed",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q in %q", want, prompt)
		}
	}

	if strings.Contains(prompt, "update ticket state/comments") {
		t.Fatalf("prompt should not tell implementer to own ticket state: %q", prompt)
	}
}
