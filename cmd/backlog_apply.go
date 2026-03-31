package cmd

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/applyspec"
	"github.com/leomorpho/docket/internal/artifacts"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var backlogApplySpecPath string
var backlogApplyWorklistPath string
var backlogApplyParent string
var backlogApplyAllowEmptyStartable bool

var tktIDPattern = regexp.MustCompile(`^TKT-\d+$`)

type backlogApplyOutput struct {
	CreatedIDs   map[string]string `json:"created_ids"`
	CreatedOrder []string          `json:"created_order"`
	NextActions  []string          `json:"next_actions"`
}

var backlogApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Create a backlog transactionally from a single spec",
	RunE: func(cmd *cobra.Command, args []string) (runErr error) {
		defer func() {
			backlogApplySpecPath = ""
			backlogApplyWorklistPath = ""
			backlogApplyParent = ""
			backlogApplyAllowEmptyStartable = false
			if f := cmd.Flags().Lookup("spec"); f != nil {
				f.Changed = false
			}
			if f := cmd.Flags().Lookup("worklist"); f != nil {
				f.Changed = false
			}
			if f := cmd.Flags().Lookup("parent"); f != nil {
				f.Changed = false
			}
			if f := cmd.Flags().Lookup("allow-empty-startable-leaf"); f != nil {
				f.Changed = false
			}
		}()
		defer func() {
			runErr = renderMutationError(cmd, runErr)
		}()

		if strings.TrimSpace(backlogApplySpecPath) != "" && strings.TrimSpace(backlogApplyWorklistPath) != "" {
			return fmt.Errorf("--spec cannot be combined with --worklist")
		}
		cfg, err := ticket.LoadConfig(repo)
		if err != nil {
			return err
		}

		var spec applyspec.BacklogApplySpec
		if strings.TrimSpace(backlogApplyWorklistPath) != "" {
			spec, err = buildBacklogSpecFromWorklist(cmd, backlogApplyWorklistPath, backlogApplyParent)
			if err != nil {
				return err
			}
		} else {
			if strings.TrimSpace(backlogApplySpecPath) == "" {
				return fmt.Errorf("either --spec or --worklist is required")
			}
			raw, err := readTicketApplySpec(cmd, backlogApplySpecPath)
			if err != nil {
				return err
			}
			parsed, report, err := applyspec.ParseBacklogSpecWithStates(raw, applyAllowedStates(cfg))
			if err != nil {
				return fmt.Errorf("parse spec JSON: %w", err)
			}
			if !report.Valid() {
				field := ""
				if len(report.Errors) > 0 {
					field = report.Errors[0].Path
				}
				return renderMutationValidationError(cmd, fmt.Errorf("backlog apply spec validation failed"), field, report)
			}
			spec = parsed
		}

		res, err := executeBacklogApply(context.Background(), repo, cfg, spec, backlogApplyAllowEmptyStartable)
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

func executeBacklogApply(ctx context.Context, repoRoot string, cfg *ticket.Config, spec applyspec.BacklogApplySpec, allowEmptyStartable bool) (backlogApplyOutput, error) {
	s := local.New(repoRoot)
	beforeComponents, err := currentComponentCount(ctx, ticketRepoRoot(repoRoot))
	if err != nil {
		return backlogApplyOutput{}, fmt.Errorf("checking graph health before apply: %w", err)
	}
	beforeWorkableCount, err := workableStartableLeafCount(ctx, s, cfg)
	if err != nil {
		return backlogApplyOutput{}, err
	}

	allocations, rollback, err := reserveBacklogIDs(repoRoot, len(spec.Tickets), s)
	if err != nil {
		return backlogApplyOutput{}, err
	}

	now := time.Now().UTC().Truncate(time.Second)
	actor := detectActor()
	plannedParentIDs, plannedParentRefs := plannedBacklogParentTargets(spec)
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
				if _, ok := plannedParentIDs[d]; ok {
					if err := rollback(createdFiles); err != nil {
						return backlogApplyOutput{}, fmt.Errorf("execution blocker %q must be a leaf ticket and rollback failed: %v", d, err)
					}
					return backlogApplyOutput{}, fmt.Errorf("execution blocker %q must be a leaf ticket and cannot be a coordination ticket", d)
				}
				blockedBy = append(blockedBy, d)
				continue
			}
			if _, ok := plannedParentRefs[d]; ok {
				if err := rollback(createdFiles); err != nil {
					return backlogApplyOutput{}, fmt.Errorf("execution blocker %q must be a leaf ticket and rollback failed: %v", d, err)
				}
				return backlogApplyOutput{}, fmt.Errorf("execution blocker %q must be a leaf ticket and cannot be a coordination ticket", d)
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

		if err := enforceCreateConnectivity(ctx, s, t); err != nil {
			if rbErr := rollback(createdFiles); rbErr != nil {
				return backlogApplyOutput{}, fmt.Errorf("%v (rollback failed: %v)", err, rbErr)
			}
			return backlogApplyOutput{}, err
		}
		if err := enforceLeafExecutionBlockers(ctx, s, t.BlockedBy); err != nil {
			if rbErr := rollback(createdFiles); rbErr != nil {
				return backlogApplyOutput{}, fmt.Errorf("%v (rollback failed: %v)", err, rbErr)
			}
			return backlogApplyOutput{}, err
		}
		if err := s.CreateTicket(ctx, t); err != nil {
			if rbErr := rollback(createdFiles); rbErr != nil {
				return backlogApplyOutput{}, fmt.Errorf("creating %s failed: %v (rollback failed: %v)", id, err, rbErr)
			}
			return backlogApplyOutput{}, fmt.Errorf("creating %s failed: %w", id, err)
		}
		createdFiles = append(createdFiles, artifacts.RepoPath(repoRoot, artifacts.RepoTicketsDir, id+".md"))
	}
	if err := enforceMutationConnectivity(ctx, ticketRepoRoot(repoRoot), beforeComponents); err != nil {
		if rbErr := rollback(createdFiles); rbErr != nil {
			return backlogApplyOutput{}, fmt.Errorf("%v (rollback failed: %v)", err, rbErr)
		}
		return backlogApplyOutput{}, err
	}
	if err := enforceStartableLeafInvariantDelta(ctx, s, cfg, allowEmptyStartable, beforeWorkableCount); err != nil {
		if rbErr := rollback(createdFiles); rbErr != nil {
			return backlogApplyOutput{}, fmt.Errorf("%v (rollback failed: %v)", err, rbErr)
		}
		return backlogApplyOutput{}, err
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

func plannedBacklogParentTargets(spec applyspec.BacklogApplySpec) (map[string]struct{}, map[string]struct{}) {
	ids := make(map[string]struct{})
	refs := make(map[string]struct{})
	for _, entry := range spec.Tickets {
		if parentID := strings.TrimSpace(entry.Parent); parentID != "" {
			ids[parentID] = struct{}{}
		}
		if parentRef := strings.TrimSpace(entry.ParentRef); parentRef != "" {
			refs[parentRef] = struct{}{}
		}
	}
	return ids, refs
}

func buildBacklogSpecFromWorklist(cmd *cobra.Command, path, parent string) (applyspec.BacklogApplySpec, error) {
	raw, err := readTicketApplySpec(cmd, path)
	if err != nil {
		return applyspec.BacklogApplySpec{}, err
	}
	parent = strings.TrimSpace(parent)
	lines := strings.Split(string(raw), "\n")
	tickets := make([]applyspec.BacklogTicketSpec, 0, len(lines))
	for i, line := range lines {
		title := normalizeWorklistTitle(line)
		if title == "" {
			continue
		}
		tickets = append(tickets, applyspec.BacklogTicketSpec{
			Ref:         "item-" + strconv.Itoa(i+1),
			Title:       title,
			Description: "Draft ticket imported from worklist. Refine details during grooming.",
			Parent:      parent,
			State:       "backlog",
		})
	}
	if len(tickets) == 0 {
		return applyspec.BacklogApplySpec{}, fmt.Errorf("worklist did not contain any ticket titles")
	}
	return applyspec.BacklogApplySpec{
		Version: applyspec.SchemaVersionV1,
		Tickets: tickets,
	}, nil
}

func normalizeWorklistTitle(line string) string {
	title := strings.TrimSpace(line)
	if title == "" {
		return ""
	}
	title = strings.TrimLeft(title, "-* \t")
	if dot := strings.Index(title, "."); dot > 0 {
		isNumbered := true
		for _, ch := range title[:dot] {
			if ch < '0' || ch > '9' {
				isNumbered = false
				break
			}
		}
		if isNumbered {
			title = strings.TrimSpace(title[dot+1:])
		}
	}
	return strings.TrimSpace(title)
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
	manifestPath := artifacts.RepoPath(repoRoot, artifacts.RepoManifest)
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
	backlogApplyCmd.Flags().StringVar(&backlogApplyWorklistPath, "worklist", "", "worklist file path (use - for stdin)")
	backlogApplyCmd.Flags().StringVar(&backlogApplyParent, "parent", "", "parent ticket ID for all worklist-created drafts")
	addAllowEmptyStartableLeafFlag(backlogApplyCmd, &backlogApplyAllowEmptyStartable)
	backlogCmd.AddCommand(backlogApplyCmd)
}
