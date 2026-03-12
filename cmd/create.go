package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	title        string
	desc         string
	priority     int
	labels       string
	state        string
	createAC     []string
	noACDefaults bool
	acTemplate   string
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new ticket",
	RunE: func(cmd *cobra.Command, args []string) error {
		defer func() {
			resetCreateGlobals()
			resetCreateFlagChanges(cmd)
		}()

		if title == "" {
			return fmt.Errorf("--title is required")
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
		if strings.TrimSpace(desc) == "" {
			return fmt.Errorf("--desc is required. Provide context with --desc \"...\" or --desc - to read from stdin")
		}
		if len(strings.TrimSpace(desc)) < 50 {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: description is under 50 characters. Add more context so another agent can execute this without clarification.\n")
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
		for _, ac := range createAC {
			trimmed := strings.TrimSpace(ac)
			if trimmed == "" {
				continue
			}
			t.AC = append(t.AC, ticket.AcceptanceCriterion{Description: trimmed})
		}
		t.AC = append(t.AC, applyTemplates(repo, acTemplate)...)
		if len(t.AC) == 0 && !noACDefaults {
			t.AC = append(t.AC, autoACDefaults(repo)...)
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
			if len(t.AC) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "  Tip: add acceptance criteria: docket ac add %s --desc \"specific testable outcome\"\n", t.ID)
			}
		}
		return nil
	},
}

func resetCreateGlobals() {
	title = ""
	desc = ""
	priority = 0
	labels = ""
	state = ""
	createAC = nil
	noACDefaults = false
	acTemplate = ""
}

func resetCreateFlagChanges(cmd *cobra.Command) {
	for _, name := range []string{"title", "desc", "priority", "labels", "state", "ac", "no-ac-defaults", "ac-template"} {
		if f := cmd.Flags().Lookup(name); f != nil {
			f.Changed = false
		}
	}
}

func init() {
	createCmd.Flags().StringVar(&title, "title", "", "ticket title (required)")
	createCmd.Flags().StringVar(&desc, "desc", "", "ticket description (use - for stdin)")
	createCmd.Flags().IntVar(&priority, "priority", 0, "ticket priority (default from config)")
	createCmd.Flags().StringVar(&labels, "labels", "", "comma-separated labels")
	createCmd.Flags().StringVar(&state, "state", "", "initial state (default from config)")
	createCmd.Flags().StringSliceVar(&createAC, "ac", []string{}, "add acceptance criteria inline (repeatable)")
	createCmd.Flags().BoolVar(&noACDefaults, "no-ac-defaults", false, "skip automatic AC defaults inferred from project stack")
	createCmd.Flags().StringVar(&acTemplate, "ac-template", "", "comma-separated AC template names to apply")

	rootCmd.AddCommand(createCmd)
}
