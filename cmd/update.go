package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/claim"
	"github.com/leomorpho/docket/internal/hooks"
	"github.com/leomorpho/docket/internal/security"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/leomorpho/docket/internal/vcs"
	"github.com/leomorpho/docket/internal/workflow"
	"github.com/spf13/cobra"
)

var (
	updateState        string
	updatePriority     int
	updateTitle        string
	updateLabels       string
	updateAddLabels    []string
	updateRemoveLabels []string
	updateBlockedBy    []string
	updateUnblock      []string
	updateParent       string
	updateCascade      bool
	updateDesc         string
	updateHandoff      string
	updatePrivTicket   string
	updatePrivYes      bool
)

var updateCmd = &cobra.Command{
	Use:   "update <TKT-NNN>",
	Short: "Update ticket fields",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer func() {
			resetUpdateGlobals()
			resetUpdateFlagChanges(cmd)
		}()

		id := args[0]
		s := local.New(repo)
		ctx := context.Background()

		t, err := s.GetTicket(ctx, id)
		if err != nil {
			return fmt.Errorf("getting ticket: %w", err)
		}
		if t == nil {
			return fmt.Errorf("ticket %s not found", id)
		}
		cfg, cfgErr := ticket.LoadConfig(repo)
		if cfgErr != nil {
			return cfgErr
		}

		var updatedFields []string

		// 1. Title
		if cmd.Flags().Changed("title") {
			nextTitle := strings.TrimSpace(updateTitle)
			if nextTitle == "" {
				return fmt.Errorf("title cannot be empty")
			}
			t.Title = nextTitle
			updatedFields = append(updatedFields, "title")
		}

		// 2. State
		if cmd.Flags().Changed("state") {
			nextState := strings.TrimSpace(updateState)
			if nextState == "" {
				return fmt.Errorf("state cannot be empty")
			}
			if !cfg.IsValidState(nextState) {
				return fmt.Errorf("%q is not a valid state", nextState)
			}
			if nextState == "stale" {
				openChildren, err := openDescendants(ctx, s, cfg, t.ID)
				if err != nil {
					return err
				}
				if len(openChildren) > 0 && !updateCascade {
					ids := make([]string, 0, len(openChildren))
					for _, c := range openChildren {
						ids = append(ids, c.ID)
					}
					sort.Strings(ids)
					return fmt.Errorf("cannot set %s to stale while open child tickets exist: %s (use --cascade)", t.ID, strings.Join(ids, ", "))
				}
				if updateCascade {
					for _, c := range openChildren {
						if err := ticket.ValidateTransition(cfg, c.State, ticket.State(nextState)); err != nil {
							return fmt.Errorf("cannot cascade state to %s: %w", c.ID, err)
						}
					}
				}
			}
			newState := ticket.State(nextState)
			if newState == "in-review" || newState == "done" {
				if err := enforceManagedRunCommitLinkage(t.ID, newState); err != nil {
					return err
				}
			}
			if newState == "done" {
				if err := enforceStructuredACClosureGate(t); err != nil {
					return err
				}
			}
			if newState == "done" || newState == "archived" {
				if err := requirePrivilegedSurface(cmd, updatePrivTicket, "state transition "+t.ID+" -> "+string(newState), updatePrivYes); err != nil {
					return err
				}
				if err := runPrivilegedHooks(cmd, t.ID, string(newState)); err != nil {
					return err
				}
			}
			if err := ticket.ValidateTransition(cfg, t.State, newState); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated %s: state %s → %s\n", t.ID, t.State, newState)

			// Use WorkflowManager for complex transitions (in-progress, done, etc.)
			vcsProv := vcs.NewGitProvider(repo)
			claimMgr := claim.NewLocalClaimManager(repo)
			wf := workflow.NewManager(s, vcsProv, claimMgr)

			if newState == "in-progress" {
				actor := detectActor()
				if agentID := os.Getenv("DOCKET_AGENT_ID"); agentID != "" {
					actor = "agent:" + agentID
				}
				_, _, err := wf.StartTask(ctx, t.ID, actor, cfg)
				if err != nil {
					return fmt.Errorf("starting task: %w", err)
				}
				// Reload ticket after StartTask
				t, _ = s.GetTicket(ctx, t.ID)
			} else if newState == "done" || newState == "archived" {
				_, err := wf.FinishTask(ctx, t.ID, cfg)
				if err != nil {
					return fmt.Errorf("finishing task: %w", err)
				}
				// Reload ticket after FinishTask
				t, _ = s.GetTicket(ctx, t.ID)
			} else {
				t.State = newState
			}
			updatedFields = append(updatedFields, "state")

			if newState == ticket.State("in-review") || newState == ticket.State("done") {
				_ = releaseLockForTicket(repo, t.ID)
			}
		}

		// 2b. Parent
		if cmd.Flags().Changed("parent") {
			p := strings.TrimSpace(updateParent)
			if p == "" || strings.EqualFold(p, "none") {
				t.Parent = ""
			} else {
				t.Parent = p
			}
			updatedFields = append(updatedFields, "parent")
		}

		// 3. Priority
		if cmd.Flags().Changed("priority") {
			t.Priority = updatePriority
			updatedFields = append(updatedFields, "priority")
		}

		// 4. Description
		if cmd.Flags().Changed("desc") {
			value, err := readUpdateValue(updateDesc)
			if err != nil {
				return err
			}
			t.Description = value
			updatedFields = append(updatedFields, "description")
		}

		// 5. Handoff
		if cmd.Flags().Changed("handoff") {
			value, err := readUpdateValue(updateHandoff)
			if err != nil {
				return err
			}
			t.Handoff = value
			updatedFields = append(updatedFields, "handoff")
		}

		// 6. Labels (replacement)
		if cmd.Flags().Changed("labels") {
			var labelList []string
			if updateLabels != "" {
				for _, l := range strings.Split(updateLabels, ",") {
					labelList = append(labelList, strings.TrimSpace(l))
				}
			}
			t.Labels = labelList
			updatedFields = append(updatedFields, "labels")
		}

		// 7. Add labels
		if len(updateAddLabels) > 0 {
			for _, l := range updateAddLabels {
				found := false
				for _, existing := range t.Labels {
					if existing == l {
						found = true
						break
					}
				}
				if !found {
					t.Labels = append(t.Labels, l)
				}
			}
			updatedFields = append(updatedFields, "add-label")
		}

		// 8. Remove labels
		if len(updateRemoveLabels) > 0 {
			var newLabels []string
			for _, existing := range t.Labels {
				remove := false
				for _, toRemove := range updateRemoveLabels {
					if existing == toRemove {
						remove = true
						break
					}
				}
				if !remove {
					newLabels = append(newLabels, existing)
				}
			}
			t.Labels = newLabels
			updatedFields = append(updatedFields, "remove-label")
		}

		// 9. Blocked by (add)
		if len(updateBlockedBy) > 0 {
			for _, b := range updateBlockedBy {
				found := false
				for _, existing := range t.BlockedBy {
					if existing == b {
						found = true
						break
					}
				}
				if !found {
					t.BlockedBy = append(t.BlockedBy, b)
				}
			}
			updatedFields = append(updatedFields, "blocked-by")
		}

		// 10. Unblock (remove)
		if len(updateUnblock) > 0 {
			var newBlockedBy []string
			for _, existing := range t.BlockedBy {
				remove := false
				for _, toRemove := range updateUnblock {
					if existing == toRemove {
						remove = true
						break
					}
				}
				if !remove {
					newBlockedBy = append(newBlockedBy, existing)
				}
			}
			t.BlockedBy = newBlockedBy
			updatedFields = append(updatedFields, "unblock")
		}

		if len(updatedFields) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No fields updated.")
			return nil
		}

		t.UpdatedAt = time.Now().UTC().Truncate(time.Second)

		if err := s.UpdateTicket(ctx, t); err != nil {
			return fmt.Errorf("updating ticket: %w", err)
		}
		if strings.TrimSpace(updateState) == "stale" && updateCascade {
			openChildren, err := openDescendants(ctx, s, cfg, t.ID)
			if err != nil {
				return err
			}
			for _, child := range openChildren {
				child.State = ticket.State(updateState)
				child.UpdatedAt = time.Now().UTC().Truncate(time.Second)
				if err := s.UpdateTicket(ctx, child); err != nil {
					return fmt.Errorf("updating child %s: %w", child.ID, err)
				}
			}
		}
		if cmd.Flags().Changed("parent") && t.Parent != "" {
			if depth, err := s.ParentDepth(ctx, t.ID); err == nil && depth > 3 {
				fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s depth is %d (>3)\n", t.ID, depth)
			}
		}

		if format == "json" {
			res := map[string]interface{}{
				"id":             t.ID,
				"updated_fields": updatedFields,
			}
			if cmd.Flags().Changed("state") {
				res["state"] = t.State
			}
			if cmd.Flags().Changed("priority") {
				res["priority"] = t.Priority
			}
			printJSON(cmd, res)
		} else if !cmd.Flags().Changed("state") {
			fmt.Fprintf(cmd.OutOrStdout(), "Updated %s: %s\n", t.ID, strings.Join(updatedFields, ", "))
		}

		return nil
	},
}

func resetUpdateGlobals() {
	updateState = ""
	updatePriority = 0
	updateTitle = ""
	updateLabels = ""
	updateAddLabels = nil
	updateRemoveLabels = nil
	updateBlockedBy = nil
	updateUnblock = nil
	updateParent = ""
	updateCascade = false
	updateDesc = ""
	updateHandoff = ""
	updatePrivTicket = ""
	updatePrivYes = false
}

func resetUpdateFlagChanges(cmd *cobra.Command) {
	flagNames := []string{
		"state",
		"priority",
		"title",
		"handoff",
		"labels",
		"add-label",
		"remove-label",
		"blocked-by",
		"unblock",
		"parent",
		"cascade",
		"desc",
		"ticket",
		"yes",
	}
	for _, name := range flagNames {
		if f := cmd.Flags().Lookup(name); f != nil {
			f.Changed = false
		}
	}
}

func readUpdateValue(value string) (string, error) {
	if value != "-" {
		return value, nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("reading from stdin: %w", err)
	}
	return string(data), nil
}

func init() {
	updateCmd.Flags().StringVar(&updateState, "state", "", "new state")
	updateCmd.Flags().IntVar(&updatePriority, "priority", 0, "new priority")
	updateCmd.Flags().StringVar(&updateTitle, "title", "", "new title")
	updateCmd.Flags().StringVar(&updateHandoff, "handoff", "", "new handoff (use - for stdin)")
	updateCmd.Flags().StringVar(&updateLabels, "labels", "", "replace all labels (csv)")
	updateCmd.Flags().StringSliceVar(&updateAddLabels, "add-label", []string{}, "add one or more labels")
	updateCmd.Flags().StringSliceVar(&updateRemoveLabels, "remove-label", []string{}, "remove one or more labels")
	updateCmd.Flags().StringSliceVar(&updateBlockedBy, "blocked-by", []string{}, "add a blocker ticket ID")
	updateCmd.Flags().StringSliceVar(&updateUnblock, "unblock", []string{}, "remove a blocker ticket ID")
	updateCmd.Flags().StringVar(&updateParent, "parent", "", "set parent ticket ID (use 'none' to clear)")
	updateCmd.Flags().BoolVar(&updateCascade, "cascade", false, "cascade state change to open descendants when required")
	updateCmd.Flags().StringVar(&updateDesc, "desc", "", "new description (use - for stdin)")
	updateCmd.Flags().StringVar(&updatePrivTicket, "ticket", "", "ticket ID authorizing privileged terminal transitions")
	updateCmd.Flags().BoolVar(&updatePrivYes, "yes", false, "skip interactive confirmation for privileged terminal transitions")

	rootCmd.AddCommand(updateCmd)
}

func openDescendants(ctx context.Context, s *local.Store, cfg *ticket.Config, id string) ([]*ticket.Ticket, error) {
	idx, err := s.BuildRelationshipIndex(ctx)
	if err != nil {
		return nil, err
	}
	openSet := map[ticket.State]bool{}
	for _, state := range cfg.OpenStates() {
		openSet[ticket.State(state)] = true
	}
	var out []*ticket.Ticket
	for _, d := range idx.Descendants(id) {
		if openSet[d.State] {
			out = append(out, d)
		}
	}
	return out, nil
}

func enforceManagedRunCommitLinkage(ticketID string, target ticket.State) error {
	ns := security.NewRepoNamespaceStore(docketHome)
	run, ok, err := ns.GetRunManifest(repo, ticketID)
	if err != nil {
		return fmt.Errorf("reading run manifest for %s: %w", ticketID, err)
	}
	if !ok {
		return nil
	}
	if err := ns.VerifyRunContext(repo, ticketID, "", "", "", ""); err != nil {
		if errors.Is(err, security.ErrRunManifestMissing) {
			return nil
		}
		return fmt.Errorf("run manifest validation failed for %s: %w", ticketID, err)
	}

	manager := hooks.NewManager()
	hooks.RegisterCoreHooks(manager)
	advisory, hookErr := manager.Run(hooks.EventReviewGate, hooks.Context{
		Repo:         repo,
		TicketID:     ticketID,
		ManagedRun:   true,
		TargetState:  string(target),
		WorktreePath: run.WorktreePath,
		Branch:       run.Branch,
		RunStartedAt: run.StartedAt,
	})
	if hookErr != nil {
		return fmt.Errorf("managed run %s cannot advance to %s: %w", ticketID, target, hookErr)
	}
	for _, msg := range advisory {
		fmt.Printf("hook advisory: %s\n", msg)
	}
	return nil
}

func runPrivilegedHooks(cmd *cobra.Command, ticketID, targetState string) error {
	manager := hooks.NewManager()
	hooks.RegisterCoreHooks(manager)
	advisory, err := manager.Run(hooks.EventPrivileged, hooks.Context{
		Repo:                 repo,
		TicketID:             ticketID,
		TargetState:          targetState,
		PrivilegedAuthorized: true,
	})
	for _, msg := range advisory {
		fmt.Fprintf(cmd.OutOrStdout(), "hook advisory: %s\n", msg)
	}
	if err != nil {
		return fmt.Errorf("privileged hook failed: %w", err)
	}
	return nil
}

func enforceStructuredACClosureGate(t *ticket.Ticket) error {
	for i, ac := range t.AC {
		kind := strings.ToLower(strings.TrimSpace(ac.NormalizedKind()))
		if kind != "human" || !ac.IsUserFacing() {
			continue
		}
		if len(ac.VerificationSteps) == 0 {
			return fmt.Errorf("cannot close %s: AC #%d requires verification_steps for human user-facing criteria", t.ID, i+1)
		}
		if !ac.Done {
			return fmt.Errorf("cannot close %s: AC #%d is human user-facing and must be marked done", t.ID, i+1)
		}
		if strings.TrimSpace(ac.Evidence) == "" {
			return fmt.Errorf("cannot close %s: AC #%d is human user-facing and requires evidence", t.ID, i+1)
		}
	}
	return nil
}
