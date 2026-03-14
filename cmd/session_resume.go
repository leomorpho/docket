package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/leomorpho/docket/internal/claim"
	"github.com/leomorpho/docket/internal/security"
	"github.com/spf13/cobra"
)

var sessionResumeCmd = &cobra.Command{
	Use:   "resume <TKT-NNN>",
	Short: "Print structured checkpoint context for agent resume",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		actor := detectActor()
		contextReset := false
		contextResetReason := ""
		if agentID := os.Getenv("DOCKET_AGENT_ID"); agentID != "" {
			actor = "agent:" + agentID
		}
		if strings.HasPrefix(actor, "agent:") {
			cl, err := claim.GetClaim(repo, id)
			if err != nil {
				return fmt.Errorf("loading claim for %s: %w", id, err)
			}
			if cl == nil || strings.TrimSpace(cl.Worktree) == "" {
				return fmt.Errorf("agent-managed resume requires a claim-bound worktree for %s", id)
			}
			absRepo, _ := filepath.Abs(repo)
			absWT, _ := filepath.Abs(cl.Worktree)
			if absWT == absRepo {
				return fmt.Errorf("agent-managed resume rejected for %s: claim points to main checkout, not dedicated worktree", id)
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			absCWD, _ := filepath.Abs(cwd)
			rel, relErr := filepath.Rel(absWT, absCWD)
			if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				return fmt.Errorf("agent-managed resume must run inside bound worktree: %s", absWT)
			}

			ns := security.NewRepoNamespaceStore(docketHome)
			activeWorkflowHash, active, err := ns.GetActiveWorkflowHash(repo)
			if err != nil {
				return fmt.Errorf("checking active workflow lock: %w", err)
			}
			expectedWorkflow := ""
			if active {
				expectedWorkflow = activeWorkflowHash
			}
			if err := ns.VerifyRunContext(repo, id, actor, cl.Worktree, "docket/"+id, expectedWorkflow); err != nil {
				if errors.Is(err, security.ErrRunManifestMissing) {
					return fmt.Errorf("agent-managed resume requires run manifest for %s", id)
				}
				return err
			}
			runManifest, ok, err := ns.GetRunManifest(repo, id)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("agent-managed resume requires run manifest for %s", id)
			}
			contextReset, contextResetReason, err = ns.UpdateContextBinding(repo, actor, id, cl.Worktree, runManifest.StartedAt)
			if err != nil {
				return fmt.Errorf("updating context binding: %w", err)
			}
		}

		paths, err := listCheckpointPaths(repo, id)
		if err != nil {
			return err
		}
		if len(paths) == 0 {
			return fmt.Errorf("no checkpoints found for %s", id)
		}
		latest := paths[len(paths)-1]
		data, err := os.ReadFile(latest)
		if err != nil {
			return err
		}
		var cp checkpoint
		if err := json.Unmarshal(data, &cp); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "RESUME_CONTEXT\n")
		fmt.Fprintf(cmd.OutOrStdout(), "ticket=%s\n", cp.TicketID)
		fmt.Fprintf(cmd.OutOrStdout(), "created_at=%s\n", cp.CreatedAt)
		fmt.Fprintf(cmd.OutOrStdout(), "ac=%d/%d\n", cp.ACDone, cp.ACTotal)
		fmt.Fprintf(cmd.OutOrStdout(), "branch=%s\n", cp.Branch)
		fmt.Fprintf(cmd.OutOrStdout(), "worktree=%s\n", cp.WorktreePath)
		fmt.Fprintf(cmd.OutOrStdout(), "changed_files=[%s]\n", strings.Join(cp.ChangedFiles, ", "))
		fmt.Fprintf(cmd.OutOrStdout(), "linked_commits=[%s]\n", strings.Join(cp.LinkedCommits, ", "))
		fmt.Fprintf(cmd.OutOrStdout(), "blockers=[%s]\n", strings.Join(cp.Blockers, ", "))
		fmt.Fprintf(cmd.OutOrStdout(), "next_steps=[%s]\n", strings.Join(cp.NextSteps, " | "))
		fmt.Fprintf(cmd.OutOrStdout(), "context_reset=%t\n", contextReset)
		if contextResetReason != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "context_reset_reason=%s\n", contextResetReason)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "last_comments=[%s]\n", strings.Join(cp.LastComments, " | "))
		if strings.TrimSpace(cp.Summary) != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "summary=%s\n", strings.TrimSpace(cp.Summary))
		}
		return nil
	},
}

func init() {
	sessionCmd.AddCommand(sessionResumeCmd)
}
