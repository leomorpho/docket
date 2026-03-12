package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show <TKT-NNN>",
	Short: "Show ticket details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		s := local.New(repo)
		ctx := context.Background()

		if format == "md" {
			raw, err := s.GetRaw(ctx, id)
			if err != nil {
				return fmt.Errorf("getting raw ticket: %w", err)
			}
			if raw == "" {
				return fmt.Errorf("ticket %s not found", id)
			}
			fmt.Fprint(cmd.OutOrStdout(), raw)
			return nil
		}

		t, err := s.GetTicket(ctx, id)
		if err != nil {
			return fmt.Errorf("getting ticket: %w", err)
		}
		if t == nil {
			return fmt.Errorf("ticket %s not found", id)
		}
		agg := aggregateDescendantAC(ctx, s, id)

		switch format {
		case "json":
			printJSON(cmd, t)
		case "context":
			printTicketContext(cmd, t, agg)
		default:
			printTicketHuman(cmd, t, agg)
		}

		return nil
	},
}

type acAggregate struct {
	Descendants int
	Total       int
	Done        int
}

func printTicketHuman(cmd *cobra.Command, t *ticket.Ticket, agg acAggregate) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%s · %s · P%d · %s\n\n", t.ID, t.State, t.Priority, strings.Join(t.Labels, ", "))
	fmt.Fprintf(out, "  %s\n\n", t.Title)

	if t.Description != "" {
		fmt.Fprintln(out, "  Description")
		fmt.Fprintln(out, "  ───────────")
		lines := strings.Split(t.Description, "\n")
		for _, line := range lines {
			fmt.Fprintf(out, "  %s\n", line)
		}
		fmt.Fprintln(out)
	}

	if len(t.AC) > 0 {
		fmt.Fprintln(out, "  Acceptance Criteria")
		fmt.Fprintln(out, "  ───────────────────")
		for _, ac := range t.AC {
			box := "[ ]"
			if ac.Done {
				box = "[x]"
			}
			line := fmt.Sprintf("  %s %s", box, ac.Description)
			if ac.Evidence != "" {
				line += " — evidence: " + ac.Evidence
			}
			fmt.Fprintln(out, line)
		}
		fmt.Fprintln(out)
	}

	if len(t.Plan) > 0 {
		fmt.Fprintln(out, "  Plan")
		fmt.Fprintln(out, "  ────")
		for i, p := range t.Plan {
			fmt.Fprintf(out, "  %d. [%-7s] %s", i+1, p.Status, p.Description)
			if p.Notes != "" {
				fmt.Fprintf(out, " — %s", p.Notes)
			}
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out)
	}

	if len(t.Comments) > 0 {
		fmt.Fprintf(out, "  Comments (%d)\n", len(t.Comments))
		fmt.Fprintln(out, "  ────────────")
		for _, c := range t.Comments {
			fmt.Fprintf(out, "  %s — %s\n", c.At.Format("2006-01-02T15:04:05Z"), c.Author)
			lines := strings.Split(c.Body, "\n")
			for _, line := range lines {
				fmt.Fprintf(out, "    %s\n", line)
			}
			fmt.Fprintln(out)
		}
	}

	if t.Handoff != "" {
		fmt.Fprintln(out, "  Handoff")
		fmt.Fprintln(out, "  ───────")
		lines := strings.Split(t.Handoff, "\n")
		for _, line := range lines {
			fmt.Fprintf(out, "  %s\n", line)
		}
		fmt.Fprintln(out)
	}

	if len(t.LinkedCommits) > 0 {
		fmt.Fprintf(out, "  Linked commits: %s\n", strings.Join(t.LinkedCommits, ", "))
	}
	if agg.Descendants > 0 {
		fmt.Fprintf(out, "  Descendant AC: %d/%d done across %d descendant tickets\n", agg.Done, agg.Total, agg.Descendants)
	}
	if len(t.BlockedBy) > 0 {
		fmt.Fprintf(out, "  Blocked by: %s\n", strings.Join(t.BlockedBy, ", "))
	} else {
		fmt.Fprintln(out, "  Blocked by: (none)")
	}
	if rel := ticketRelationsLine(t.ID); rel != "" {
		fmt.Fprintf(out, "  Relations: %s\n", rel)
	}
}

func printTicketContext(cmd *cobra.Command, t *ticket.Ticket, agg acAggregate) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "TICKET: %s · %s · P%d\n", t.ID, t.State, t.Priority)
	fmt.Fprintf(out, "TITLE: %s\n", t.Title)
	if t.Description != "" {
		fmt.Fprintf(out, "DESCRIPTION: %s\n", strings.ReplaceAll(t.Description, "\n", " "))
	}

	if len(t.AC) > 0 {
		done := 0
		var remaining []string
		for _, ac := range t.AC {
			if ac.Done {
				done++
			} else {
				remaining = append(remaining, ac.Description)
			}
		}
		fmt.Fprintf(out, "AC: %d/%d done. Remaining: [%s]\n", done, len(t.AC), strings.Join(remaining, "] ["))
	}

	if len(t.Plan) > 0 {
		var planSteps []string
		for _, p := range t.Plan {
			planSteps = append(planSteps, fmt.Sprintf("%s:[%s]", p.Status, p.Description))
		}
		fmt.Fprintf(out, "PLAN: %s\n", strings.Join(planSteps, " "))
	}

	if t.Handoff != "" {
		fmt.Fprintf(out, "HANDOFF: %s\n", strings.ReplaceAll(t.Handoff, "\n", " "))
	}

	if len(t.LinkedCommits) > 0 {
		fmt.Fprintf(out, "LINKED COMMITS: %s\n", strings.Join(t.LinkedCommits, ", "))
	}
	if agg.Descendants > 0 {
		fmt.Fprintf(out, "DESCENDANT AC: %d/%d done across %d tickets\n", agg.Done, agg.Total, agg.Descendants)
	}

	if len(t.BlockedBy) > 0 {
		fmt.Fprintf(out, "BLOCKED BY: %s\n", strings.Join(t.BlockedBy, ", "))
	} else {
		fmt.Fprintln(out, "BLOCKED BY: none")
	}
	if rel := ticketRelationsLine(t.ID); rel != "" {
		fmt.Fprintf(out, "RELATIONS: %s\n", rel)
	}
}

func ticketRelationsLine(id string) string {
	st, err := loadRelations(repo)
	if err != nil || len(st.Relations) == 0 {
		return ""
	}
	parts := []string{}
	for _, r := range st.Relations {
		if r.From == id {
			parts = append(parts, fmt.Sprintf("%s -> %s", r.Relation, r.To))
		}
		if r.To == id {
			parts = append(parts, fmt.Sprintf("%s <- %s", r.Relation, r.From))
		}
	}
	return strings.Join(parts, "; ")
}

func aggregateDescendantAC(ctx context.Context, s *local.Store, id string) acAggregate {
	idx, err := s.BuildRelationshipIndex(ctx)
	if err != nil {
		return acAggregate{}
	}
	desc := idx.Descendants(id)
	if len(desc) == 0 {
		return acAggregate{}
	}
	agg := acAggregate{Descendants: len(desc)}
	for _, d := range desc {
		full, err := s.GetTicket(ctx, d.ID)
		if err != nil || full == nil {
			continue
		}
		agg.Total += len(full.AC)
		for _, ac := range full.AC {
			if ac.Done {
				agg.Done++
			}
		}
	}
	return agg
}

func init() {
	rootCmd.AddCommand(showCmd)
}
