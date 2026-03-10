package tui

import (
	"fmt"
	"strings"

	"github.com/leoaudibert/docket/internal/ticket"
)

func formatTicketDetail(t *ticket.Ticket) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s · %s · P%d\n\n", t.ID, t.State, t.Priority)
	fmt.Fprintf(&b, "%s\n\n", t.Title)

	if t.Description != "" {
		b.WriteString("Description\n")
		b.WriteString("-----------\n")
		for _, line := range strings.Split(t.Description, "\n") {
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(t.AC) > 0 {
		b.WriteString("Acceptance Criteria\n")
		b.WriteString("-------------------\n")
		for _, ac := range t.AC {
			box := "[ ]"
			if ac.Done {
				box = "[x]"
			}
			line := fmt.Sprintf("%s %s", box, ac.Description)
			if ac.Evidence != "" {
				line += " (evidence: " + ac.Evidence + ")"
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(t.Plan) > 0 {
		b.WriteString("Plan\n")
		b.WriteString("----\n")
		for i, p := range t.Plan {
			fmt.Fprintf(&b, "%d. [%-7s] %s", i+1, p.Status, p.Description)
			if p.Notes != "" {
				b.WriteString(" - ")
				b.WriteString(p.Notes)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(t.Comments) > 0 {
		fmt.Fprintf(&b, "Comments (%d)\n", len(t.Comments))
		b.WriteString("------------\n")
		for _, c := range t.Comments {
			fmt.Fprintf(&b, "%s - %s\n", c.At.Format("2006-01-02T15:04:05Z"), c.Author)
			for _, line := range strings.Split(c.Body, "\n") {
				b.WriteString("  ")
				b.WriteString(line)
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}

	if len(t.LinkedCommits) > 0 {
		b.WriteString("Linked commits: ")
		b.WriteString(strings.Join(t.LinkedCommits, ", "))
		b.WriteString("\n")
	}

	if len(t.BlockedBy) > 0 {
		b.WriteString("Blocked by: ")
		b.WriteString(strings.Join(t.BlockedBy, ", "))
		b.WriteString("\n")
	} else {
		b.WriteString("Blocked by: (none)\n")
	}

	return strings.TrimSpace(b.String())
}
