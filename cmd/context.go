package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	g "github.com/leoaudibert/docket/internal/git"
	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var contextLines string

type contextTicket struct {
	Ticket      *ticket.Ticket
	Lines       []int
	CommitLines map[string]int
}

var contextCmd = &cobra.Command{
	Use:   "context <file>",
	Short: "Show ticket context for a file from git history and annotations",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		file := args[0]
		start, end, err := parseLineRange(contextLines)
		if err != nil {
			return err
		}

		entries, err := g.BlameFile(repo, file, start, end)
		if err != nil {
			return err
		}

		shaToTicket := map[string]string{}
		for _, e := range entries {
			if _, ok := shaToTicket[e.SHA]; ok {
				continue
			}
			tid, err := g.CommitTicket(repo, e.SHA)
			if err != nil {
				return err
			}
			shaToTicket[e.SHA] = tid
		}

		s := local.New(repo)
		ticketsMap := map[string]*contextTicket{}
		for _, e := range entries {
			tid := shaToTicket[e.SHA]
			if tid == "" {
				continue
			}
			ct, ok := ticketsMap[tid]
			if !ok {
				t, err := s.GetTicket(context.Background(), tid)
				if err != nil {
					return err
				}
				if t == nil {
					continue
				}
				ct = &contextTicket{Ticket: t, CommitLines: map[string]int{}}
				ticketsMap[tid] = ct
			}
			ct.Lines = append(ct.Lines, e.Line)
			if _, exists := ct.CommitLines[e.SHA]; !exists {
				ct.CommitLines[e.SHA] = e.Line
			}
		}

		relFile := filepath.ToSlash(file)
		if filepath.IsAbs(file) {
			if rel, relErr := filepath.Rel(repo, file); relErr == nil {
				relFile = filepath.ToSlash(rel)
			}
		}
		annotations, err := s.GetAnnotationsByFile(context.Background(), relFile)
		if err != nil {
			return err
		}

		var tickets []*contextTicket
		for _, ct := range ticketsMap {
			ct.Lines = uniqueSortedInts(ct.Lines)
			tickets = append(tickets, ct)
		}
		sort.SliceStable(tickets, func(i, j int) bool {
			li := tickets[i].Lines[len(tickets[i].Lines)-1]
			lj := tickets[j].Lines[len(tickets[j].Lines)-1]
			return li > lj
		})

		switch format {
		case "json":
			printContextJSON(cmd, relFile, tickets, annotations)
		case "context":
			printContextCompact(cmd, relFile, tickets, annotations)
		default:
			printContextHuman(cmd, relFile, tickets, annotations)
		}
		return nil
	},
}

func parseLineRange(v string) (int, int, error) {
	if strings.TrimSpace(v) == "" {
		return 0, 0, nil
	}
	parts := strings.Split(v, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("--lines must be in <start>-<end> format")
	}
	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || start <= 0 {
		return 0, 0, fmt.Errorf("invalid start line: %s", parts[0])
	}
	end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || end < start {
		return 0, 0, fmt.Errorf("invalid end line: %s", parts[1])
	}
	return start, end, nil
}

func printContextHuman(cmd *cobra.Command, file string, tickets []*contextTicket, anns []local.Annotation) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Context for %s\n\n", file)

	if len(tickets) == 0 {
		fmt.Fprintln(out, "No tickets linked to this file's history")
	} else {
		fmt.Fprintln(out, "Tickets from git history:")
		for _, ct := range tickets {
			t := ct.Ticket
			fmt.Fprintf(out, "  %s (%s, P%d) — %s\n", t.ID, t.State, t.Priority, t.Title)
			fmt.Fprintf(out, "    Lines: %s\n", formatLineSpan(ct.Lines))
			fmt.Fprintf(out, "    AC: %d/%d done.\n\n", acDone(t), len(t.AC))
		}
	}

	if len(anns) > 0 {
		fmt.Fprintln(out, "Inline annotations:")
		for _, a := range anns {
			fmt.Fprintf(out, "  Line %d: [%s] %s\n", a.LineNum, a.TicketID, strings.TrimSpace(a.Context))
		}
	}
}

func printContextCompact(cmd *cobra.Command, file string, tickets []*contextTicket, anns []local.Annotation) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "FILE: %s\n", file)
	fmt.Fprintln(out, "TICKETS:")
	for _, ct := range tickets {
		t := ct.Ticket
		fmt.Fprintf(out, "  %s %s P%d | %s\n", t.ID, t.State, t.Priority, t.Title)
		fmt.Fprintf(out, "    HANDOFF: %s\n", oneLine(t.Handoff))
		fmt.Fprintf(out, "    AC: %d/%d\n", acDone(t), len(t.AC))
	}
	fmt.Fprintln(out, "ANNOTATIONS:")
	for _, a := range anns {
		fmt.Fprintf(out, "  L%d: [%s] %s\n", a.LineNum, a.TicketID, strings.TrimSpace(a.Context))
	}
}

func printContextJSON(cmd *cobra.Command, file string, tickets []*contextTicket, anns []local.Annotation) {
	ticketEntries := make([]map[string]interface{}, 0, len(tickets))
	for _, ct := range tickets {
		t := ct.Ticket
		ticketEntries = append(ticketEntries, map[string]interface{}{
			"id":            t.ID,
			"title":         t.Title,
			"state":         t.State,
			"handoff":       t.Handoff,
			"ac_status":     map[string]int{"total": len(t.AC), "done": acDone(t)},
			"lines_touched": ct.Lines,
		})
	}

	annotationEntries := make([]map[string]interface{}, 0, len(anns))
	for _, a := range anns {
		annotationEntries = append(annotationEntries, map[string]interface{}{
			"line":      a.LineNum,
			"ticket_id": a.TicketID,
			"context":   strings.TrimSpace(a.Context),
		})
	}

	printJSON(cmd, map[string]interface{}{
		"file":        file,
		"tickets":     ticketEntries,
		"annotations": annotationEntries,
	})
}

func acDone(t *ticket.Ticket) int {
	done := 0
	for _, ac := range t.AC {
		if ac.Done {
			done++
		}
	}
	return done
}

func oneLine(s string) string {
	trim := strings.TrimSpace(s)
	if trim == "" {
		return "(none)"
	}
	return strings.Join(strings.Fields(trim), " ")
}

func formatLineSpan(lines []int) string {
	if len(lines) == 0 {
		return "(none)"
	}
	return fmt.Sprintf("%d-%d", lines[0], lines[len(lines)-1])
}

func uniqueSortedInts(v []int) []int {
	if len(v) == 0 {
		return v
	}
	sort.Ints(v)
	out := []int{v[0]}
	for i := 1; i < len(v); i++ {
		if v[i] != v[i-1] {
			out = append(out, v[i])
		}
	}
	return out
}

func init() {
	contextCmd.Flags().StringVar(&contextLines, "lines", "", "optional line range as start-end")
	rootCmd.AddCommand(contextCmd)
}
