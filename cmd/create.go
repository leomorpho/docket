package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	title    string
	desc     string
	priority int
	labels   string
	state    string
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new ticket",
	RunE: func(cmd *cobra.Command, args []string) error {
		if title == "" {
			return fmt.Errorf("--title is required")
		}

		// Quality warnings (advisory only — never block creation)
		if desc == "" || desc == "-" {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: --desc not provided. Add context so other agents can pick up this ticket without asking questions.\n  Tip: docket update TKT-NNN --desc \"...\"\n")
		} else if len(strings.Fields(desc)) < 20 {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: description is very short. Add enough context for another agent to execute this without clarification.\n")
		}

		cfg, err := ticket.LoadConfig(repo)
		if err != nil {
			return err
		}

		// Apply config defaults when flags are not explicitly set.
		if !cmd.Flags().Changed("state") {
			state = cfg.DefaultState
		}
		if !cmd.Flags().Changed("priority") {
			priority = cfg.DefaultPriority
		}

		s := local.New(repo)
		ctx := context.Background()

		// 1. Get next ID
		id, seq, err := s.NextID(ctx)
		if err != nil {
			return fmt.Errorf("getting next ID: %w", err)
		}

		// 2. Detect actor
		actor := detectActor()

		// 3. Handle description from stdin
		if desc == "-" {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("reading from stdin: %w", err)
			}
			desc = string(data)
		}

		// 4. Parse labels
		var labelList []string
		if labels != "" {
			for l := range strings.SplitSeq(labels, ",") {
				labelList = append(labelList, strings.TrimSpace(l))
			}
		}

		// 5. Validate state
		if !cfg.IsValidState(state) {
			return fmt.Errorf("%q is not a valid state. Valid states: %s", state, strings.Join(cfg.StateNames(), ", "))
		}

		now := time.Now().UTC().Truncate(time.Second)
		t := &ticket.Ticket{
			ID:          id,
			Seq:         seq,
			Title:       title,
			Description: desc,
			Priority:    priority,
			Labels:      labelList,
			State:       ticket.State(state),
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   actor,
		}

		// 6. Create ticket
		if err := s.CreateTicket(ctx, t); err != nil {
			return fmt.Errorf("creating ticket: %w", err)
		}

		// 7. Output
		if format == "json" {
			printJSON(cmd, map[string]any{
				"id":       t.ID,
				"seq":      t.Seq,
				"title":    t.Title,
				"state":    t.State,
				"priority": t.Priority,
			})
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Created %s: %s\n", t.ID, t.Title)
			fmt.Fprintf(cmd.OutOrStdout(), "  Tip: add acceptance criteria: docket ac add %s --desc \"specific testable outcome\"\n", t.ID)
		}
		return nil
	},
}

func init() {
	createCmd.Flags().StringVar(&title, "title", "", "ticket title (required)")
	createCmd.Flags().StringVar(&desc, "desc", "", "ticket description (use - for stdin)")
	createCmd.Flags().IntVar(&priority, "priority", 0, "ticket priority (default from config)")
	createCmd.Flags().StringVar(&labels, "labels", "", "comma-separated labels")
	createCmd.Flags().StringVar(&state, "state", "", "initial state (default from config)")

	rootCmd.AddCommand(createCmd)
}
