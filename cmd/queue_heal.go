package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	workablepkg "github.com/leomorpho/docket/internal/workable"
	"github.com/spf13/cobra"
)

var queueHealApply bool

type queueHealResult struct {
	Healthy          bool     `json:"healthy"`
	Applied          bool     `json:"applied"`
	TicketID         string   `json:"ticket_id,omitempty"`
	RemovedBlocker   string   `json:"removed_blocker,omitempty"`
	RemainingBlocker []string `json:"remaining_blockers,omitempty"`
	Summary          string   `json:"summary"`
	NextActions      []string `json:"next_actions,omitempty"`
}

var queueHealCmd = &cobra.Command{
	Use:   "heal",
	Short: "Suggest or apply a minimal unblock to restore a workable startable leaf ticket",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		cfg, err := ticket.LoadConfig(repo)
		if err != nil {
			return err
		}
		s := local.New(repo)
		res, err := executeQueueHeal(ctx, s, cfg, queueHealApply)
		if err != nil {
			return err
		}

		if format == "json" {
			printJSON(cmd, res)
			return nil
		}
		fmt.Fprintln(cmd.OutOrStdout(), res.Summary)
		for _, next := range res.NextActions {
			fmt.Fprintf(cmd.OutOrStdout(), "Next: %s\n", next)
		}
		return nil
	},
}

func executeQueueHeal(ctx context.Context, s *local.Store, cfg *ticket.Config, apply bool) (queueHealResult, error) {
	// Healthy queue: no-op.
	workableCount, err := workableStartableLeafCount(ctx, s, cfg)
	if err != nil {
		return queueHealResult{}, err
	}
	if workableCount > 0 {
		return queueHealResult{
			Healthy: true,
			Applied: false,
			Summary: "Queue invariant already healthy: at least one workable startable leaf ticket exists.",
		}, nil
	}

	target, unresolved, err := pickBlockedStartableLeaf(ctx, s, cfg)
	if err != nil {
		return queueHealResult{}, err
	}
	if target == nil {
		diagnosis, _ := workablepkg.DiagnoseEmpty(ctx, s, cfg)
		return queueHealResult{
			Healthy: false,
			Applied: false,
			Summary: diagnosis.Summary(),
			NextActions: []string{
				"Create or unblock at least one startable topo:leaf ticket.",
				"Use `docket list --state backlog --full --label topo:leaf` to inspect candidates.",
			},
		}, nil
	}

	res := queueHealResult{
		Healthy:  false,
		Applied:  false,
		TicketID: target.ID,
		Summary: fmt.Sprintf("Suggested minimal heal: unblock %s by removing blocker %s.",
			target.ID, unresolved[0]),
		NextActions: []string{
			fmt.Sprintf("docket update %s --unblock %s", target.ID, unresolved[0]),
			"docket list --state open --format context",
		},
	}

	if !apply {
		return res, nil
	}

	original := *target
	original.BlockedBy = append([]string(nil), target.BlockedBy...)

	removed := unresolved[0]
	filtered := make([]string, 0, len(target.BlockedBy))
	dropped := false
	for _, blockerID := range target.BlockedBy {
		if !dropped && blockerID == removed {
			dropped = true
			continue
		}
		filtered = append(filtered, blockerID)
	}
	target.BlockedBy = filtered
	target.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	if err := s.UpdateTicket(ctx, target); err != nil {
		return queueHealResult{}, fmt.Errorf("applying queue heal on %s: %w", target.ID, err)
	}
	if err := enforceStartableLeafInvariant(ctx, s, cfg, false); err != nil {
		original.UpdatedAt = time.Now().UTC().Truncate(time.Second)
		if rollbackErr := s.UpdateTicket(ctx, &original); rollbackErr != nil {
			return queueHealResult{}, fmt.Errorf("%v; rollback failed: %w", err, rollbackErr)
		}
		return queueHealResult{}, err
	}
	remaining, _ := s.UnresolvedBlockers(ctx, target)
	res.Applied = true
	res.RemovedBlocker = removed
	res.RemainingBlocker = remaining
	res.Summary = fmt.Sprintf("Applied queue heal: removed blocker %s from %s and restored a workable queue.", removed, target.ID)
	res.NextActions = []string{
		fmt.Sprintf("docket show %s --format context", target.ID),
		"docket list --state open --format context",
	}
	return res, nil
}

func pickBlockedStartableLeaf(ctx context.Context, s *local.Store, cfg *ticket.Config) (*ticket.Ticket, []string, error) {
	startable := cfg.StartableStates()
	if len(startable) == 0 {
		return nil, nil, nil
	}
	filter := store.Filter{States: make([]ticket.State, 0, len(startable))}
	for _, st := range startable {
		filter.States = append(filter.States, ticket.State(st))
	}
	items, err := s.ListTickets(ctx, filter)
	if err != nil {
		return nil, nil, err
	}

	type candidate struct {
		ticket    *ticket.Ticket
		blockers  []string
		createdAt time.Time
	}
	candidates := make([]candidate, 0, len(items))
	for _, item := range items {
		full, err := s.GetTicket(ctx, item.ID)
		if err != nil {
			return nil, nil, err
		}
		if !workablepkg.IsTicket(cfg, full) {
			continue
		}
		unresolved, err := s.UnresolvedBlockers(ctx, full)
		if err != nil {
			return nil, nil, err
		}
		if len(unresolved) == 0 {
			continue
		}
		candidates = append(candidates, candidate{
			ticket:    full,
			blockers:  unresolved,
			createdAt: full.CreatedAt,
		})
	}
	if len(candidates) == 0 {
		return nil, nil, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ticket.Priority != candidates[j].ticket.Priority {
			return candidates[i].ticket.Priority < candidates[j].ticket.Priority
		}
		if !candidates[i].createdAt.Equal(candidates[j].createdAt) {
			return candidates[i].createdAt.Before(candidates[j].createdAt)
		}
		return strings.Compare(candidates[i].ticket.ID, candidates[j].ticket.ID) < 0
	})
	return candidates[0].ticket, candidates[0].blockers, nil
}

func init() {
	queueHealCmd.Flags().BoolVar(&queueHealApply, "apply", false, "apply a minimal unblock fix automatically")
	queueCmd.AddCommand(queueHealCmd)
}
