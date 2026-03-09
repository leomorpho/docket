package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	commentBody string
)

var commentCmd = &cobra.Command{
	Use:   "comment <TKT-NNN>",
	Short: "Add a comment to a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		if commentBody == "" {
			return fmt.Errorf("--body is required")
		}

		s := local.New(repo)
		ctx := context.Background()

		// 1. Detect actor
		actor := detectActor()

		// 2. Handle body from stdin
		if commentBody == "-" {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("reading from stdin: %w", err)
			}
			commentBody = string(data)
		}

		now := time.Now().UTC().Truncate(time.Second)
		c := ticket.Comment{
			At:     now,
			Author: actor,
			Body:   commentBody,
		}

		// 3. Add comment
		if err := s.AddComment(ctx, id, c); err != nil {
			return fmt.Errorf("adding comment: %w", err)
		}

		// 4. Output
		if format == "json" {
			printJSON(cmd, map[string]string{
				"ticket_id": id,
				"at":        c.At.Format(time.RFC3339),
				"author":    c.Author,
			})
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Comment added to %s.\n", id)
		}

		// Reset global variables for test isolation
		commentBody = ""

		return nil
	},
}

func init() {
	commentCmd.Flags().StringVar(&commentBody, "body", "", "comment text (use - for stdin)")
	rootCmd.AddCommand(commentCmd)
}
