package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/leomorpho/docket/internal/claim"
	"github.com/leomorpho/docket/internal/security"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/leomorpho/docket/internal/vcs"
	"github.com/leomorpho/docket/internal/workflow"
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
		ns := security.NewRepoNamespaceStore(docketHome)
		activeWorkflowHash, active, err := ns.GetActiveWorkflowHash(repo)
		if err != nil {
			return fmt.Errorf("checking active workflow policy: %w", err)
		}
		if !active {
			return fmt.Errorf("no active workflow.lock is approved for this repo. Run `docket workflow lock activate --ticket TKT-NNN` in secure mode")
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

		// 2. Start the ticket using WorkflowManager
		vcsProv := vcs.NewGitProvider(repo)
		claimMgr := claim.NewLocalClaimManager(repo)
		wf := workflow.NewManager(s, vcsProv, claimMgr)

		actor := detectActor()
		if agentID := os.Getenv("DOCKET_AGENT_ID"); agentID != "" {
			actor = "agent:" + agentID
		}

		t, worktreePath, err := wf.StartTask(ctx, t.ID, actor, cfg)
		if err != nil {
			return err
		}
		if worktreePath == "" {
			worktreePath = repo
		}
		if err := ns.RecordRunStart(repo, t.ID, actor, worktreePath, "docket/"+t.ID, activeWorkflowHash); err != nil {
			return fmt.Errorf("recording run manifest: %w", err)
		}

		// 3. Provide the Agent Prompt
		if format == "json" {
			printJSON(cmd, map[string]interface{}{
				"ticket":            t,
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

func init() {
	startCmd.Flags().BoolVar(&startAuto, "auto", false, "automatically continue to the next ticket after completion")
	rootCmd.AddCommand(startCmd)
}
