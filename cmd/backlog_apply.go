package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/applyspec"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var backlogApplySpecPath string

var tktIDPattern = regexp.MustCompile(`^TKT-\d+$`)

type backlogApplyOutput struct {
	CreatedIDs   map[string]string `json:"created_ids"`
	CreatedOrder []string          `json:"created_order"`
	NextActions  []string          `json:"next_actions"`
}

var backlogApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Create a backlog transactionally from a single spec",
	RunE: func(cmd *cobra.Command, args []string) error {
		defer func() {
			backlogApplySpecPath = ""
			if f := cmd.Flags().Lookup("spec"); f != nil {
				f.Changed = false
			}
		}()

		if strings.TrimSpace(backlogApplySpecPath) == "" {
			return fmt.Errorf("--spec is required")
		}
		raw, err := readTicketApplySpec(cmd, backlogApplySpecPath)
		if err != nil {
			return err
		}
		spec, report, err := applyspec.ParseBacklogSpec(raw)
		if err != nil {
			return fmt.Errorf("parse spec JSON: %w", err)
		}
		if !report.Valid() {
			if format == "json" {
				printJSON(cmd, map[string]any{
					"error":      "validation_failed",
					"validation": report,
				})
			}
			return fmt.Errorf("backlog apply spec validation failed")
		}

		res, err := executeBacklogApply(context.Background(), repo, spec)
		if err != nil {
			return err
		}

		if format == "json" {
			printJSON(cmd, res)
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Created %d tickets\n", len(res.CreatedOrder))
		for ref, id := range res.CreatedIDs {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s -> %s\n", ref, id)
		}
		if len(res.NextActions) > 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "Next actions:")
			for _, next := range res.NextActions {
				fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", next)
			}
		}
		return nil
	},
}

func executeBacklogApply(ctx context.Context, repoRoot string, spec applyspec.BacklogApplySpec) (backlogApplyOutput, error) {
	s := local.New(repoRoot)
	cfg, err := ticket.LoadConfig(repoRoot)
	if err != nil {
		return backlogApplyOutput{}, err
	}

	allocations, rollback, err := reserveBacklogIDs(repoRoot, len(spec.Tickets), s)
	if err != nil {
		return backlogApplyOutput{}, err
	}

	now := time.Now().UTC().Truncate(time.Second)
	actor := detectActor()
	createdIDs := map[string]string{}
	createdOrder := make([]string, 0, len(spec.Tickets))
	createdFiles := make([]string, 0, len(spec.Tickets))

	for i, entry := range spec.Tickets {
		if strings.TrimSpace(entry.ID) != "" {
			if err := rollback(createdFiles); err != nil {
				return backlogApplyOutput{}, fmt.Errorf("backlog apply currently supports create-only specs; rollback failed: %v", err)
			}
			return backlogApplyOutput{}, fmt.Errorf("backlog apply currently supports create-only specs; omit tickets[%d].id", i)
		}
		id := allocations[i].id
		seq := allocations[i].seq
		if entry.Ref != "" {
			createdIDs[entry.Ref] = id
		}
		createdOrder = append(createdOrder, id)

		parentID := strings.TrimSpace(entry.Parent)
		if parentID == "" && strings.TrimSpace(entry.ParentRef) != "" {
			mapped, ok := createdIDs[entry.ParentRef]
			if !ok {
				if err := rollback(createdFiles); err != nil {
					return backlogApplyOutput{}, fmt.Errorf("resolve parent_ref %q failed and rollback failed: %v", entry.ParentRef, err)
				}
				return backlogApplyOutput{}, fmt.Errorf("unable to resolve parent_ref %q", entry.ParentRef)
			}
			parentID = mapped
		}

		blockedBy := make([]string, 0, len(entry.BlockedBy))
		for _, dep := range entry.BlockedBy {
			d := strings.TrimSpace(dep)
			if d == "" {
				continue
			}
			if tktIDPattern.MatchString(d) {
				blockedBy = append(blockedBy, d)
				continue
			}
			mapped, ok := createdIDs[d]
			if !ok {
				if err := rollback(createdFiles); err != nil {
					return backlogApplyOutput{}, fmt.Errorf("resolve blocked_by %q failed and rollback failed: %v", d, err)
				}
				return backlogApplyOutput{}, fmt.Errorf("unable to resolve blocked_by reference %q", d)
			}
			blockedBy = append(blockedBy, mapped)
		}

		state := ticket.State(cfg.DefaultState)
		if strings.TrimSpace(entry.State) != "" {
			state = ticket.State(entry.State)
		}
		priority := cfg.DefaultPriority
		if entry.Priority != nil {
			priority = *entry.Priority
		}

		t := &ticket.Ticket{
			ID:          id,
			Seq:         seq,
			State:       state,
			Priority:    priority,
			Labels:      append([]string(nil), entry.Labels...),
			Parent:      parentID,
			BlockedBy:   blockedBy,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   actor,
			Title:       entry.Title,
			Description: entry.Description,
			AC:          acceptanceCriteriaFromSpec(entry.AC),
		}

		if err := s.CreateTicket(ctx, t); err != nil {
			if rbErr := rollback(createdFiles); rbErr != nil {
				return backlogApplyOutput{}, fmt.Errorf("creating %s failed: %v (rollback failed: %v)", id, err, rbErr)
			}
			return backlogApplyOutput{}, fmt.Errorf("creating %s failed: %w", id, err)
		}
		createdFiles = append(createdFiles, filepath.Join(repoRoot, ".docket", "tickets", id+".md"))
	}

	return backlogApplyOutput{
		CreatedIDs:   createdIDs,
		CreatedOrder: createdOrder,
		NextActions: []string{
			"docket list --state backlog",
			fmt.Sprintf("docket show %s", createdOrder[0]),
		},
	}, nil
}

type idAllocation struct {
	id  string
	seq int
}

func reserveBacklogIDs(repoRoot string, count int, s *local.Store) ([]idAllocation, func(createdFiles []string) error, error) {
	cfgPath := ticket.ConfigPath(repoRoot)
	cfgSnapshot, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read config snapshot: %w", err)
	}
	manifestPath := filepath.Join(repoRoot, ".docket", "manifest.json")
	manifestSnapshot, manifestExists, err := readOptionalFile(manifestPath)
	if err != nil {
		return nil, nil, err
	}

	cfg, err := ticket.LoadConfig(repoRoot)
	if err != nil {
		return nil, nil, err
	}
	start := cfg.Counter + 1
	cfg.Counter += count
	if err := ticket.SaveConfig(repoRoot, cfg); err != nil {
		return nil, nil, fmt.Errorf("reserve IDs in config: %w", err)
	}

	allocations := make([]idAllocation, 0, count)
	for i := 0; i < count; i++ {
		seq := start + i
		allocations = append(allocations, idAllocation{id: ticket.FormatID(seq), seq: seq})
	}

	rollback := func(createdFiles []string) error {
		for _, path := range createdFiles {
			_ = os.Remove(path)
		}
		if err := os.WriteFile(cfgPath, cfgSnapshot, 0o644); err != nil {
			return err
		}
		if manifestExists {
			if err := os.WriteFile(manifestPath, manifestSnapshot, 0o644); err != nil {
				return err
			}
		} else {
			_ = os.Remove(manifestPath)
		}
		s.InvalidateRelationshipIndex()
		return nil
	}

	return allocations, rollback, nil
}

func readOptionalFile(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}

func init() {
	backlogApplyCmd.Flags().StringVar(&backlogApplySpecPath, "spec", "", "spec file path (use - for stdin)")
	backlogCmd.AddCommand(backlogApplyCmd)
}
