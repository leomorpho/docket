package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

const (
	contextOptimizeMaxItems   = 3
	contextOptimizeBriefChars = 280
	contextOptimizeItemChars  = 180
)

type contextOptimizeRecentActivity struct {
	Comments          []string `json:"comments"`
	LinkedCommits     []string `json:"linked_commits"`
	CheckpointSummary string   `json:"checkpoint_summary,omitempty"`
}

type contextOptimizeOutput struct {
	TicketID       string                        `json:"ticket_id"`
	Title          string                        `json:"title"`
	State          ticket.State                  `json:"state"`
	Brief          string                        `json:"brief"`
	ACStatus       map[string]int                `json:"ac_status"`
	RelatedWork    []string                      `json:"related_work"`
	LearningRules  []startLearnRule              `json:"learning_rules"`
	RecentActivity contextOptimizeRecentActivity `json:"recent_activity"`
	NextSteps      []string                      `json:"next_steps"`
}

var contextOptimizeCmd = &cobra.Command{
	Use:     "context-optimize <TKT-NNN>",
	Aliases: []string{"brief"},
	Short:   "Build a compact task brief from ticket context sources",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := strings.TrimSpace(args[0])
		s := local.New(repo)
		t, err := s.GetTicket(context.Background(), id)
		if err != nil {
			return err
		}
		if t == nil {
			return fmt.Errorf("ticket %s not found", id)
		}

		payload, err := buildContextOptimizeOutput(context.Background(), repo, s, t)
		if err != nil {
			return err
		}
		if format == "json" {
			printJSON(cmd, payload)
			return nil
		}
		renderContextOptimizeHuman(cmd, payload)
		return nil
	},
}

func buildContextOptimizeOutput(ctx context.Context, repoRoot string, s *local.Store, t *ticket.Ticket) (contextOptimizeOutput, error) {
	cfg, err := ticket.LoadConfig(repoRoot)
	if err != nil {
		cfg = ticket.DefaultConfig()
	}
	related := collectContextOptimizeRelatedWork(ctx, s, cfg, t, contextOptimizeMaxItems)
	learnRules := buildLearnReplay(repoRoot, t, contextOptimizeMaxItems)
	recent := collectContextOptimizeRecentActivity(repoRoot, t, contextOptimizeMaxItems)
	nextSteps := collectContextOptimizeNextSteps(t, contextOptimizeMaxItems)

	return contextOptimizeOutput{
		TicketID:       t.ID,
		Title:          t.Title,
		State:          t.State,
		Brief:          truncateWithEllipsis(oneLine(t.Description), contextOptimizeBriefChars),
		ACStatus:       map[string]int{"done": acDone(t), "total": len(t.AC)},
		RelatedWork:    related,
		LearningRules:  learnRules,
		RecentActivity: recent,
		NextSteps:      nextSteps,
	}, nil
}

func collectContextOptimizeRelatedWork(ctx context.Context, s *local.Store, cfg *ticket.Config, t *ticket.Ticket, limit int) []string {
	items := make([]string, 0, limit)
	if strings.TrimSpace(t.Parent) != "" {
		if parent, err := s.GetTicket(ctx, t.Parent); err == nil && parent != nil {
			items = append(items, fmt.Sprintf("Parent %s (%s): %s", parent.ID, parent.State, truncateWithEllipsis(parent.Title, contextOptimizeItemChars)))
		} else {
			items = append(items, fmt.Sprintf("Parent %s", t.Parent))
		}
	}
	for _, blocker := range t.BlockedBy {
		items = append(items, fmt.Sprintf("Blocked by %s", strings.TrimSpace(blocker)))
	}
	for _, blocked := range t.Blocks {
		items = append(items, fmt.Sprintf("Blocks %s", strings.TrimSpace(blocked)))
	}
	if strings.TrimSpace(t.Parent) != "" {
		if all, err := s.ListTickets(ctx, store.Filter{}); err == nil {
			for _, candidate := range all {
				if candidate.ID == t.ID || candidate.Parent != t.Parent {
					continue
				}
				if cfg.StateHasRole(string(candidate.State), "completed") || cfg.StateHasRole(string(candidate.State), "archived") {
					continue
				}
				items = append(items, fmt.Sprintf("Sibling %s (%s): %s", candidate.ID, candidate.State, truncateWithEllipsis(candidate.Title, contextOptimizeItemChars)))
			}
		}
	}
	return dedupeAndLimit(items, limit)
}

func collectContextOptimizeRecentActivity(repoRoot string, t *ticket.Ticket, limit int) contextOptimizeRecentActivity {
	comments := make([]string, 0, limit)
	start := len(t.Comments) - limit
	if start < 0 {
		start = 0
	}
	for _, c := range t.Comments[start:] {
		body := strings.TrimSpace(c.Body)
		if body == "" {
			continue
		}
		comments = append(comments, fmt.Sprintf("%s: %s", strings.TrimSpace(c.Author), truncateWithEllipsis(oneLine(body), contextOptimizeItemChars)))
	}
	linkedCommits := tailAndTrimStrings(t.LinkedCommits, limit)

	checkpointSummary := ""
	if paths, err := listCheckpointPaths(repoRoot, t.ID); err == nil && len(paths) > 0 {
		data, err := os.ReadFile(paths[len(paths)-1])
		if err == nil {
			var cp checkpoint
			if json.Unmarshal(data, &cp) == nil {
				checkpointSummary = truncateWithEllipsis(oneLine(strings.TrimSpace(cp.Summary)), contextOptimizeItemChars)
			}
		}
	}

	return contextOptimizeRecentActivity{
		Comments:          comments,
		LinkedCommits:     linkedCommits,
		CheckpointSummary: checkpointSummary,
	}
}

func collectContextOptimizeNextSteps(t *ticket.Ticket, limit int) []string {
	next := make([]string, 0, limit)
	for _, ac := range t.AC {
		if ac.Done {
			continue
		}
		desc := strings.TrimSpace(ac.Description)
		if desc == "" {
			continue
		}
		next = append(next, truncateWithEllipsis(desc, contextOptimizeItemChars))
	}
	return dedupeAndLimit(next, limit)
}

func tailAndTrimStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) == 0 {
		return nil
	}
	start := len(values) - limit
	if start < 0 {
		start = 0
	}
	out := make([]string, 0, limit)
	for _, value := range values[start:] {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, truncateWithEllipsis(trimmed, contextOptimizeItemChars))
	}
	return out
}

func dedupeAndLimit(values []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, limit)
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func truncateWithEllipsis(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func renderContextOptimizeHuman(cmd *cobra.Command, out contextOptimizeOutput) {
	fmt.Fprintf(cmd.OutOrStdout(), "Context brief for %s\n", out.TicketID)
	fmt.Fprintf(cmd.OutOrStdout(), "Title: %s\n", out.Title)
	fmt.Fprintf(cmd.OutOrStdout(), "State: %s\n", out.State)
	fmt.Fprintf(cmd.OutOrStdout(), "Brief: %s\n", out.Brief)
	fmt.Fprintf(cmd.OutOrStdout(), "AC: %d/%d done\n", out.ACStatus["done"], out.ACStatus["total"])
	fmt.Fprintln(cmd.OutOrStdout(), "")

	fmt.Fprintln(cmd.OutOrStdout(), "Related work:")
	if len(out.RelatedWork) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "  - none")
	} else {
		for _, line := range out.RelatedWork {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", line)
		}
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Learning rules:")
	if len(out.LearningRules) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "  - none")
	} else {
		for _, rule := range out.LearningRules {
			fmt.Fprintf(cmd.OutOrStdout(), "  - [%s] %s\n", rule.Category, rule.Rule)
		}
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Recent activity:")
	if len(out.RecentActivity.Comments) == 0 && len(out.RecentActivity.LinkedCommits) == 0 && strings.TrimSpace(out.RecentActivity.CheckpointSummary) == "" {
		fmt.Fprintln(cmd.OutOrStdout(), "  - none")
	} else {
		for _, comment := range out.RecentActivity.Comments {
			fmt.Fprintf(cmd.OutOrStdout(), "  - comment: %s\n", comment)
		}
		for _, commit := range out.RecentActivity.LinkedCommits {
			fmt.Fprintf(cmd.OutOrStdout(), "  - commit: %s\n", commit)
		}
		if strings.TrimSpace(out.RecentActivity.CheckpointSummary) != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  - checkpoint: %s\n", out.RecentActivity.CheckpointSummary)
		}
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Next steps:")
	if len(out.NextSteps) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "  - none")
	} else {
		for _, step := range out.NextSteps {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", step)
		}
	}
}

func init() {
	rootCmd.AddCommand(contextOptimizeCmd)
}
