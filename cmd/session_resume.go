package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	runruntime "github.com/leomorpho/docket/internal/agentrun/runtime"
	"github.com/leomorpho/docket/internal/claim"
	"github.com/leomorpho/docket/internal/runstate"
	"github.com/leomorpho/docket/internal/store/local"
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

			ns := runstate.New(runtimeNamespaceRoot(repo))
			activeWorkflowHash, active, err := ns.GetActiveWorkflowHash(repo)
			if err != nil {
				return fmt.Errorf("checking active runtime policy pack: %w", err)
			}
			expectedWorkflow := ""
			if active {
				expectedWorkflow = activeWorkflowHash
			}
			if err := ns.VerifyRunContext(repo, id, actor, cl.Worktree, "docket/"+id, expectedWorkflow); err != nil {
				if errors.Is(err, runstate.ErrRunManifestMissing) {
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

		var cp checkpoint
		paths, err := listCheckpointPaths(repo, id)
		if err != nil {
			return err
		}
		if len(paths) > 0 {
			latest := paths[len(paths)-1]
			data, err := os.ReadFile(latest)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(data, &cp); err != nil {
				return err
			}
		} else {
			brief, ok, err := runruntime.New(repo).LoadBrief(id)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("no checkpoints or managed-run brief found for %s", id)
			}
			cp, err = buildResumeCheckpointFromBrief(repo, id, brief)
			if err != nil {
				return err
			}
		}

		fmt.Fprintf(cmd.OutOrStdout(), "RESUME_CONTEXT\n")
		fmt.Fprintf(cmd.OutOrStdout(), "ticket=%s\n", cp.TicketID)
		if strings.TrimSpace(cp.TicketState) != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "state=%s\n", cp.TicketState)
		}
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

func buildResumeCheckpointFromBrief(repoRoot, ticketID string, brief runruntime.RunBrief) (checkpoint, error) {
	s := local.New(repoRoot)
	tkt, err := s.GetTicket(context.Background(), ticketID)
	if err != nil {
		return checkpoint{}, err
	}
	cp := checkpoint{
		TicketID:     ticketID,
		CreatedAt:    strings.TrimSpace(brief.UpdatedAt),
		LastComments: []string{},
		Summary:      strings.TrimSpace(brief.Summary),
	}
	if cp.CreatedAt == "" {
		cp.CreatedAt = brief.UpdatedAt
	}
	if tkt != nil {
		cp.TicketState = strings.TrimSpace(string(tkt.State))
		cp.ACTotal = len(tkt.AC)
		for _, ac := range tkt.AC {
			if ac.Done {
				cp.ACDone++
			} else if strings.TrimSpace(ac.Description) != "" {
				cp.NextSteps = append(cp.NextSteps, strings.TrimSpace(ac.Description))
			}
		}
		cp.LinkedCommits = append(cp.LinkedCommits, tkt.LinkedCommits...)
		cp.Blockers = append(cp.Blockers, tkt.BlockedBy...)
		if len(tkt.Comments) > 0 {
			start := len(tkt.Comments) - 3
			if start < 0 {
				start = 0
			}
			for _, c := range tkt.Comments[start:] {
				cp.LastComments = append(cp.LastComments, strings.TrimSpace(c.Body))
			}
		}
	}
	if strings.TrimSpace(brief.CommitSHA) != "" && !containsStringValue(cp.LinkedCommits, brief.CommitSHA) {
		cp.LinkedCommits = append(cp.LinkedCommits, brief.CommitSHA)
	}
	if len(brief.FilesTouched) > 0 {
		cp.ChangedFiles = append(cp.ChangedFiles, brief.FilesTouched...)
	} else {
		cp.ChangedFiles = gitChangedFiles(repoRoot)
	}
	if strings.TrimSpace(brief.ResumeNext) != "" {
		cp.NextSteps = append(cp.NextSteps, brief.ResumeNext)
	}
	if strings.TrimSpace(brief.Tests) != "" {
		cp.LastComments = append(cp.LastComments, "Validation: "+strings.TrimSpace(brief.Tests))
	}
	if len(brief.ValidationErrors) > 0 {
		cp.LastComments = append(cp.LastComments, "Validation errors: "+strings.Join(brief.ValidationErrors, "; "))
	}
	cp.Branch = gitCurrentBranch(repoRoot)
	cp.WorktreePath = repoRoot
	if strings.TrimSpace(cp.CreatedAt) == "" {
		cp.CreatedAt = brief.UpdatedAt
	}
	if strings.TrimSpace(cp.CreatedAt) == "" {
		cp.CreatedAt = "unknown"
	}
	return cp, nil
}

func containsStringValue(values []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func init() {
	sessionCmd.AddCommand(sessionResumeCmd)
}
