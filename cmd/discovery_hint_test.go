package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestShouldEmitGlobalSkillHint_ScaffoldCommandsAreExcluded(t *testing.T) {
	root := &cobra.Command{Use: "docket"}
	ticket := &cobra.Command{Use: "ticket"}
	ticketScaffold := &cobra.Command{Use: "scaffold"}
	backlog := &cobra.Command{Use: "backlog"}
	backlogScaffold := &cobra.Command{Use: "scaffold"}
	list := &cobra.Command{Use: "list"}

	root.AddCommand(ticket, backlog, list)
	ticket.AddCommand(ticketScaffold)
	backlog.AddCommand(backlogScaffold)

	if shouldEmitGlobalSkillHint(ticketScaffold, "human") {
		t.Fatalf("expected ticket scaffold to suppress global skill hint")
	}
	if shouldEmitGlobalSkillHint(backlogScaffold, "human") {
		t.Fatalf("expected backlog scaffold to suppress global skill hint")
	}
	if !shouldEmitGlobalSkillHint(list, "human") {
		t.Fatalf("expected non-scaffold human command to emit global skill hint")
	}
	if shouldEmitGlobalSkillHint(list, "json") {
		t.Fatalf("expected json output to suppress global skill hint")
	}
}
