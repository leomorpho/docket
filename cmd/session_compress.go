package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	sessionCompressName    string
	sessionCompressKeep    bool
	sessionCompressSummary string
)

var sessionCompressCmd = &cobra.Command{
	Use:   "compress <TKT-NNN>",
	Short: "Compress a session into a handoff summary",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		s := local.New(repo)
		ctx := context.Background()

		sessionPath, err := s.ResolveSessionPath(ctx, id, sessionCompressName)
		if err != nil {
			return err
		}

		rawSession, err := os.ReadFile(sessionPath)
		if err != nil {
			return err
		}

		if strings.TrimSpace(sessionCompressSummary) == "" {
			fmt.Fprintln(cmd.OutOrStdout(), "Write a handoff summary in this format and rerun with --summary-file <path>:")
			fmt.Fprintln(cmd.OutOrStdout(), "")
			fmt.Fprintln(cmd.OutOrStdout(), "## Handoff")
			fmt.Fprintf(cmd.OutOrStdout(), "*Last updated: %s by %s*\n\n", time.Now().UTC().Format("2006-01-02T15:04:05Z"), detectActor())
			fmt.Fprintln(cmd.OutOrStdout(), "**Current state:**")
			fmt.Fprintln(cmd.OutOrStdout(), "**Decisions made:**")
			fmt.Fprintln(cmd.OutOrStdout(), "**Files touched:**")
			fmt.Fprintln(cmd.OutOrStdout(), "**Remaining work:**")
			fmt.Fprintln(cmd.OutOrStdout(), "**AC status:**")
			fmt.Fprintln(cmd.OutOrStdout(), "")
			fmt.Fprintln(cmd.OutOrStdout(), "Session excerpt:")
			fmt.Fprintln(cmd.OutOrStdout(), string(rawSession))
			return nil
		}

		summaryData, err := os.ReadFile(sessionCompressSummary)
		if err != nil {
			return err
		}
		summary := strings.TrimSpace(string(summaryData))
		summary = strings.TrimSpace(strings.TrimPrefix(summary, "## Handoff"))

		t, err := s.GetTicket(ctx, id)
		if err != nil {
			return err
		}
		if t == nil {
			return fmt.Errorf("ticket %s not found", id)
		}
		t.Handoff = summary
		t.UpdatedAt = time.Now().UTC().Truncate(time.Second)
		if err := s.UpdateTicket(ctx, t); err != nil {
			return err
		}

		c := ticket.Comment{
			At:     time.Now().UTC().Truncate(time.Second),
			Author: detectActor(),
			Body:   "Session compressed. Handoff updated.",
		}
		if err := s.AddComment(ctx, id, c); err != nil {
			return err
		}

		finalPath := sessionPath
		if !sessionCompressKeep {
			finalPath, err = s.MarkSessionCompressed(sessionPath)
			if err != nil {
				return err
			}
		}

		relFinal := filepath.ToSlash(finalPath)
		if rel, relErr := filepath.Rel(s.RepoRoot, finalPath); relErr == nil {
			relFinal = filepath.ToSlash(rel)
		}

		if format == "json" {
			printJSON(cmd, map[string]string{"ticket_id": id, "session": relFinal, "status": "compressed"})
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Session compressed for %s. Handoff updated.\n", id)
		}
		return nil
	},
}

func init() {
	sessionCompressCmd.Flags().StringVar(&sessionCompressName, "session", "", "session filename (default: latest)")
	sessionCompressCmd.Flags().BoolVar(&sessionCompressKeep, "keep", false, "keep original session filename without .compressed rename")
	sessionCompressCmd.Flags().StringVar(&sessionCompressSummary, "summary-file", "", "path to handoff summary markdown")
	sessionCmd.AddCommand(sessionCompressCmd)
}
