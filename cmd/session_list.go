package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/spf13/cobra"
)

var sessionListCmd = &cobra.Command{
	Use:   "list <TKT-NNN>",
	Short: "List session files attached to a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		s := local.New(repo)
		files, err := s.ListSessions(context.Background(), id)
		if err != nil {
			return err
		}

		if format == "json" {
			printJSON(cmd, map[string]interface{}{"ticket_id": id, "sessions": files})
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Sessions for %s:\n", id)
		if len(files) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "  (none)")
			return nil
		}

		for _, f := range files {
			note := ""
			if strings.HasSuffix(f.Name, ".compressed") {
				note = ", compressed"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  %s (%s%s)\n", f.Name, humanize.Bytes(uint64(f.SizeBytes)), note)
		}
		return nil
	},
}

func init() {
	sessionCmd.AddCommand(sessionListCmd)
}
