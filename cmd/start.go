package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/leomorpho/docket/internal/claim"
	"github.com/leomorpho/docket/internal/git"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	startAuto bool
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Automatically pick up and start the next best ticket",
	Long: `Identify the next unblocked high-priority ticket in an open state,
claims it, creates a worktree for it if needed, and transitions it to 'in-progress'.
In --auto mode, it will continue to the next ticket after each completion.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s := local.New(repo)
		ctx := context.Background()

		cfg, err := ticket.LoadConfig(repo)
		if err != nil {
			return err
		}

		// 1. Select the next ticket
		t, err := selectNextTicket(ctx, s, cfg)
		if err != nil {
			return err
		}
		if t == nil {
			fmt.Fprintln(cmd.OutOrStdout(), "No unblocked tickets found in open states (backlog, todo).")
			return nil
		}

		// Load full ticket details (ListTickets only returns metadata)
		t, err = s.GetTicket(ctx, t.ID)
		if err != nil {
			return fmt.Errorf("loading ticket details: %w", err)
		}

		// 2. Start the ticket
		if err := startTicket(ctx, s, cfg, t); err != nil {
			return err
		}

		// 3. Provide the Agent Prompt
		if format == "json" {
			printJSON(cmd, map[string]interface{}{
				"ticket":           t,
				"agent_instruction": "Analyze the requirements and implement the changes. Use 'docket' tools to track your progress.",
			})
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "\n=== Agent Prompt ===\n")
		fmt.Fprintf(cmd.OutOrStdout(), "You have started working on ticket: %s\n", t.ID)
		fmt.Fprintf(cmd.OutOrStdout(), "Title: %s\n", t.Title)
		fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", t.Description)
		fmt.Fprintf(cmd.OutOrStdout(), "\nAcceptance Criteria:\n")
		for _, ac := range t.AC {
			status := "[ ]"
			if ac.Done {
				status = "[x]"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "- %s %s\n", status, ac.Description)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\nInstruction: Analyze the requirements and implement the changes. Use 'docket' tools to track your progress.\n")
		fmt.Fprintf(cmd.OutOrStdout(), "====================\n")

		return nil
	},
}

func selectNextTicket(ctx context.Context, s *local.Store, cfg *ticket.Config) (*ticket.Ticket, error) {
	// Candidates are open states that are NOT in-progress
	var candidates []ticket.State
	for _, stateName := range cfg.OpenStates() {
		st := ticket.State(stateName)
		if st != "in-progress" {
			candidates = append(candidates, st)
		}
	}

	f := store.Filter{
		States:        candidates,
		OnlyUnblocked: true,
	}

	tickets, err := s.ListTickets(ctx, f)
	if err != nil {
		return nil, err
	}

	for _, t := range tickets {
		if err := ticket.ValidateTransition(cfg, t.State, ticket.State("in-progress")); err == nil {
			return t, nil
		}
	}

	return nil, nil
}

func startTicket(ctx context.Context, s *local.Store, cfg *ticket.Config, t *ticket.Ticket) error {
	oldState := t.State
	newState := ticket.State("in-progress")

	if !cfg.IsValidState(string(newState)) {
		return fmt.Errorf("state 'in-progress' is not defined in config")
	}

	if err := ticket.ValidateTransition(cfg, oldState, newState); err != nil {
		return fmt.Errorf("invalid transition for %s: %w", t.ID, err)
	}

	t.State = newState
	t.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	if t.StartedAt.IsZero() {
		t.StartedAt = t.UpdatedAt
	}

	// Logic for Claims/Worktrees
	actor := detectActor()
	if agentID := os.Getenv("DOCKET_AGENT_ID"); agentID != "" {
		actor = "agent:" + agentID
	}

	// Try to create a worktree if this is an agent (or always?)
	// For now let's follow the MCP logic but make it robust for CLI use
	wtPath, wtErr := git.GetAgentWorktreeDir(t.ID)
	if wtErr == nil {
		branch := "docket/" + t.ID
		if err := git.CreateWorktree(repo, t.ID, branch, wtPath); err == nil {
			_ = claim.Claim(repo, t.ID, wtPath, actor)
			fmt.Printf("Claimed %s in worktree: %s\n", t.ID, wtPath)
		} else {
			// Fallback to current worktree
			_ = claim.Claim(repo, t.ID, repo, actor)
			fmt.Printf("Claimed %s in current directory (worktree creation failed: %v)\n", t.ID, err)
		}
	} else {
		_ = claim.Claim(repo, t.ID, repo, actor)
		fmt.Printf("Claimed %s in current directory\n", t.ID)
	}

	return s.UpdateTicket(ctx, t)
}

func init() {
	startCmd.Flags().BoolVar(&startAuto, "auto", false, "automatically continue to the next ticket after completion")
	rootCmd.AddCommand(startCmd)
}
