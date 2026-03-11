package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	g "github.com/leoaudibert/docket/internal/git"
	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/spf13/cobra"
)

var blameCmd = &cobra.Command{
	Use:   "blame <file>:<line>",
	Short: "Find the ticket linked to a line via git blame and commit trailers",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		file, line, err := parseFileLineArg(args[0])
		if err != nil {
			return err
		}

		blame, err := g.BlameLine(repo, file, line)
		if err != nil {
			return err
		}

		ticketID, err := g.CommitTicket(repo, blame.SHA)
		if err != nil {
			return err
		}

		s := local.New(repo)
		if format == "json" {
			res := map[string]interface{}{
				"file":      file,
				"line":      line,
				"commit":    blame.SHA,
				"ticket_id": ticketID,
			}
			if ticketID != "" {
				t, err := s.GetTicket(context.Background(), ticketID)
				if err != nil {
					return fmt.Errorf("loading ticket %s: %w", ticketID, err)
				}
				res["ticket"] = t
			}
			printJSON(cmd, res)
			return nil
		}

		if ticketID == "" {
			fmt.Fprintf(cmd.OutOrStdout(), "No ticket linked to commit %s.\n", shortSHA(blame.SHA))
			summary := blame.Summary
			if summary == "" {
				summary = "(no summary)"
			}
			meta := blame.Date
			if meta == "" {
				meta = "unknown date"
			}
			author := blame.Author
			if author == "" {
				author = "unknown author"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Commit: %q (%s, %s)\n", summary, meta, author)
			return nil
		}

		t, err := s.GetTicket(context.Background(), ticketID)
		if err != nil {
			return fmt.Errorf("loading ticket %s: %w", ticketID, err)
		}
		if t == nil {
			return fmt.Errorf("ticket %s linked from commit %s not found", ticketID, shortSHA(blame.SHA))
		}

		fmt.Fprintf(cmd.OutOrStdout(), "%s · %s · P%d — %s\n\n", t.ID, t.State, t.Priority, t.Title)
		commitDate := blame.Date
		if commitDate == "" {
			commitDate = "unknown date"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  This line was last modified in commit %s (%s)\n", shortSHA(blame.SHA), commitDate)
		fmt.Fprintf(cmd.OutOrStdout(), "  Ticket: %s\n\n", ticketID)
		printTicketContext(cmd, t, acAggregate{})

		return nil
	},
}

func parseFileLineArg(s string) (string, int, error) {
	idx := strings.LastIndex(s, ":")
	if idx <= 0 || idx == len(s)-1 {
		return "", 0, fmt.Errorf("argument must be in <file>:<line> format")
	}

	file := strings.TrimSpace(s[:idx])
	lineStr := strings.TrimSpace(s[idx+1:])
	line, err := strconv.Atoi(lineStr)
	if err != nil || line <= 0 {
		return "", 0, fmt.Errorf("invalid line number: %s", lineStr)
	}

	return file, line, nil
}

func shortSHA(sha string) string {
	if len(sha) <= 9 {
		return sha
	}
	return sha[:9]
}

func init() {
	rootCmd.AddCommand(blameCmd)
}
