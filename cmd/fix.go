package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/leomorpho/docket/internal/git"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	fixAll bool
)

var fixCmd = &cobra.Command{
	Use:   "fix [TKT-NNN]",
	Short: "Repair ticket signatures and teach correct tool usage",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := local.New(repo)

		if fixAll {
			return runFixAll(cmd, s)
		}

		if len(args) == 0 {
			return fmt.Errorf("ticket ID or --all is required")
		}

		return runFixOne(cmd, s, args[0])
	},
}

func runFixOne(cmd *cobra.Command, s *local.Store, id string) error {
	// 1. Load current (potentially invalid) ticket
	current, err := s.GetTicket(context.Background(), id)
	if err != nil {
		return fmt.Errorf("reading current ticket: %w", err)
	}
	if current == nil {
		return fmt.Errorf("ticket %s not found", id)
	}

	// 2. Try to load last known good from git
	relPath := filepath.Join(".docket", "tickets", id+".md")
	lastGoodData, err := git.Show(repo, "HEAD", relPath)
	var lastGood *ticket.Ticket
	if err == nil {
		lastGood, _ = local.Parse(lastGoodData)
	}

	// 3. Detect changes and build tutor message
	changes := detectChanges(lastGood, current)
	
	// 4. Re-sign and save via store (this fixes the hash)
	if err := s.UpdateTicket(context.Background(), current); err != nil {
		return fmt.Errorf("failed to save fixed ticket: %w", err)
	}

	// 5. Output Tutor Message
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Repairing %s. Do not edit markdown files directly. Use the tools.\n", id)
	if len(changes) > 0 {
		fmt.Fprintf(out, "You should have used:\n")
		for _, msg := range changes {
			fmt.Fprintf(out, "  %s\n", msg)
		}
	} else {
		fmt.Fprintf(out, "Signature was missing or corrupt, but no field changes detected.\n")
	}

	return nil
}

func runFixAll(cmd *cobra.Command, s *local.Store) error {
	ticketsDir := filepath.Join(repo, ".docket", "tickets")
	entries, err := os.ReadDir(ticketsDir)
	if err != nil {
		return err
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			id := strings.TrimSuffix(entry.Name(), ".md")
			// Only fix if invalid
			errs, _, _ := s.ValidateFile(id)
			isInvalid := false
			for _, e := range errs {
				if e.Field == "signature" {
					isInvalid = true
					break
				}
			}
			
			if isInvalid {
				if err := runFixOne(cmd, s, id); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Failed to fix %s: %v\n", id, err)
				} else {
					count++
				}
			}
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Successfully repaired %d tickets.\n", count)
	return nil
}

func detectChanges(old, new *ticket.Ticket) []string {
	var msgs []string
	if old == nil {
		return []string{fmt.Sprintf("docket create --title %q", new.Title)}
	}

	if old.Title != new.Title {
		msgs = append(msgs, fmt.Sprintf("docket update %s --title %q", new.ID, new.Title))
	}
	if old.State != new.State {
		msgs = append(msgs, fmt.Sprintf("docket update %s --state %s", new.ID, new.State))
	}
	if old.Priority != new.Priority {
		msgs = append(msgs, fmt.Sprintf("docket update %s --priority %d", new.ID, new.Priority))
	}
	if old.Parent != new.Parent {
		p := new.Parent
		if p == "" { p = "none" }
		msgs = append(msgs, fmt.Sprintf("docket update %s --parent %s", new.ID, p))
	}
	if old.Description != new.Description {
		msgs = append(msgs, fmt.Sprintf("docket update %s --desc '...'", new.ID))
	}
	
	// Complex fields like AC/Plan/Comments are harder to represent as a single command if many changed,
	// but we can give a general hint.
	if len(old.AC) != len(new.AC) {
		msgs = append(msgs, fmt.Sprintf("docket ac add %s --desc '...'", new.ID))
	}
	if len(old.Comments) != len(new.Comments) {
		msgs = append(msgs, fmt.Sprintf("docket comment %s --body '...'", new.ID))
	}

	return msgs
}

func init() {
	fixCmd.Flags().BoolVar(&fixAll, "all", false, "repair all tickets with invalid signatures")
	rootCmd.AddCommand(fixCmd)
}
