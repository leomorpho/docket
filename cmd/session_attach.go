package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var sessionAttachFile string

var sessionAttachCmd = &cobra.Command{
	Use:   "attach <TKT-NNN>",
	Short: "Attach a session log file to a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if sessionAttachFile == "" {
			return fmt.Errorf("--file is required")
		}
		id := args[0]
		s := local.New(repo)
		ctx := context.Background()

		relPath, err := s.AttachSession(ctx, id, sessionAttachFile)
		if err != nil {
			return err
		}

		c := ticket.Comment{
			At:     time.Now().UTC().Truncate(time.Second),
			Author: detectActor(),
			Body:   "Session attached: " + relPath,
		}
		if err := s.AddComment(ctx, id, c); err != nil {
			return err
		}

		if format == "json" {
			printJSON(cmd, map[string]string{"ticket_id": id, "session": relPath})
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Session attached to %s: %s\n", id, relPath)
		}

		return nil
	},
}

func init() {
	sessionAttachCmd.Flags().StringVar(&sessionAttachFile, "file", "", "path to a session log file")
	sessionCmd.AddCommand(sessionAttachCmd)
}
