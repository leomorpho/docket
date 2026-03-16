package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/applyspec"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var ticketApplySpecPath string

type ticketApplyPresence struct {
	ID          bool
	Title       bool
	Description bool
	Priority    bool
	State       bool
	Labels      bool
	Parent      bool
	BlockedBy   bool
	AC          bool
}

type ticketApplyOutput struct {
	ID          string   `json:"id"`
	Action      string   `json:"action"`
	Warnings    []string `json:"warnings,omitempty"`
	NextActions []string `json:"next_actions"`
}

var ticketApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Create or update a ticket transactionally from a spec",
	RunE: func(cmd *cobra.Command, args []string) (runErr error) {
		defer func() {
			ticketApplySpecPath = ""
			if f := cmd.Flags().Lookup("spec"); f != nil {
				f.Changed = false
			}
		}()
		defer func() {
			runErr = renderMutationError(cmd, runErr)
		}()

		if strings.TrimSpace(ticketApplySpecPath) == "" {
			return fmt.Errorf("--spec is required")
		}

		raw, err := readTicketApplySpec(cmd, ticketApplySpecPath)
		if err != nil {
			return err
		}

		spec, report, err := applyspec.ParseTicketSpec(raw)
		if err != nil {
			return fmt.Errorf("parse spec JSON: %w", err)
		}
		if !report.Valid() {
			field := ""
			if len(report.Errors) > 0 {
				field = report.Errors[0].Path
			}
			return renderMutationValidationError(cmd, fmt.Errorf("ticket apply spec validation failed"), field, report)
		}

		presence, err := parseTicketPresence(raw)
		if err != nil {
			return fmt.Errorf("parse ticket field presence: %w", err)
		}

		res, err := executeTicketApply(context.Background(), repo, spec, presence)
		if err != nil {
			return err
		}

		if format == "json" {
			printJSON(cmd, res)
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Applied %s (%s)\n", res.ID, res.Action)
		for _, warn := range res.Warnings {
			fmt.Fprintf(cmd.OutOrStdout(), "warning: %s\n", warn)
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

func executeTicketApply(ctx context.Context, repoRoot string, spec applyspec.TicketApplySpec, presence ticketApplyPresence) (ticketApplyOutput, error) {
	s := local.New(repoRoot)
	cfg, err := ticket.LoadConfig(repoRoot)
	if err != nil {
		return ticketApplyOutput{}, err
	}

	now := time.Now().UTC().Truncate(time.Second)
	actor := detectActor()
	operation := spec.Operation
	if operation == "" {
		operation = applyspec.OperationUpsert
	}

	if spec.Ticket.ID != "" {
		existing, err := s.GetTicket(ctx, spec.Ticket.ID)
		if err != nil {
			return ticketApplyOutput{}, fmt.Errorf("loading ticket %s: %w", spec.Ticket.ID, err)
		}
		if existing != nil {
			updated, warnings, err := applyUpdateTicket(existing, spec, presence, cfg)
			if err != nil {
				return ticketApplyOutput{}, err
			}
			updated.UpdatedAt = now
			if err := s.UpdateTicket(ctx, updated); err != nil {
				return ticketApplyOutput{}, fmt.Errorf("updating ticket %s: %w", updated.ID, err)
			}
			return ticketApplyOutput{
				ID:       updated.ID,
				Action:   "updated",
				Warnings: warnings,
				NextActions: []string{
					fmt.Sprintf("docket show %s", updated.ID),
					fmt.Sprintf("docket validate %s", updated.ID),
				},
			}, nil
		}
		if operation == applyspec.OperationUpdate {
			return ticketApplyOutput{}, fmt.Errorf("ticket %s not found for update", spec.Ticket.ID)
		}
		return ticketApplyOutput{}, fmt.Errorf("ticket %s does not exist; omit ticket.id to create with next ID", spec.Ticket.ID)
	}

	id, seq, rollbackCounter, err := reserveNextID(ctx, repoRoot, s)
	if err != nil {
		return ticketApplyOutput{}, err
	}
	newTicket := &ticket.Ticket{
		ID:          id,
		Seq:         seq,
		Title:       spec.Ticket.Title,
		Description: spec.Ticket.Description,
		Priority:    cfg.DefaultPriority,
		State:       ticket.State(cfg.DefaultState),
		Labels:      append([]string(nil), spec.Ticket.Labels...),
		Parent:      spec.Ticket.Parent,
		BlockedBy:   append([]string(nil), spec.Ticket.BlockedBy...),
		AC:          acceptanceCriteriaFromSpec(spec.Ticket.AC),
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   actor,
	}
	if spec.Ticket.Priority != nil {
		newTicket.Priority = *spec.Ticket.Priority
	}
	if spec.Ticket.State != "" {
		newTicket.State = ticket.State(spec.Ticket.State)
	}
	if err := s.CreateTicket(ctx, newTicket); err != nil {
		if rollbackErr := rollbackCounter(); rollbackErr != nil {
			return ticketApplyOutput{}, fmt.Errorf("creating ticket failed: %v (rollback failed: %v)", err, rollbackErr)
		}
		return ticketApplyOutput{}, fmt.Errorf("creating ticket failed: %w", err)
	}

	warnings := []string{}
	if len(newTicket.AC) == 0 {
		warnings = append(warnings, fmt.Sprintf("%s has no acceptance criteria", newTicket.ID))
	}

	return ticketApplyOutput{
		ID:       newTicket.ID,
		Action:   "created",
		Warnings: warnings,
		NextActions: []string{
			fmt.Sprintf("docket show %s", newTicket.ID),
			fmt.Sprintf("docket validate %s", newTicket.ID),
		},
	}, nil
}

func applyUpdateTicket(existing *ticket.Ticket, spec applyspec.TicketApplySpec, presence ticketApplyPresence, cfg *ticket.Config) (*ticket.Ticket, []string, error) {
	t := *existing
	warnings := []string{}

	if presence.Title {
		if strings.TrimSpace(spec.Ticket.Title) == "" {
			return nil, nil, fmt.Errorf("ticket.title cannot be empty")
		}
		t.Title = spec.Ticket.Title
	}
	if presence.Description {
		if strings.TrimSpace(spec.Ticket.Description) == "" {
			return nil, nil, fmt.Errorf("ticket.description cannot be empty")
		}
		t.Description = spec.Ticket.Description
	}
	if presence.Priority && spec.Ticket.Priority != nil {
		t.Priority = *spec.Ticket.Priority
	}
	if presence.State {
		nextState := ticket.State(spec.Ticket.State)
		if err := ticket.ValidateTransition(cfg, t.State, nextState); err != nil {
			return nil, nil, err
		}
		t.State = nextState
	}
	if presence.Labels {
		t.Labels = append([]string(nil), spec.Ticket.Labels...)
	}
	if presence.Parent {
		t.Parent = spec.Ticket.Parent
	}
	if presence.BlockedBy {
		t.BlockedBy = append([]string(nil), spec.Ticket.BlockedBy...)
	}
	if presence.AC {
		t.AC = acceptanceCriteriaFromSpec(spec.Ticket.AC)
		if len(t.AC) == 0 {
			warnings = append(warnings, fmt.Sprintf("%s has no acceptance criteria", t.ID))
		}
	}

	return &t, warnings, nil
}

func reserveNextID(ctx context.Context, repoRoot string, s *local.Store) (string, int, func() error, error) {
	cfgPath := ticket.ConfigPath(repoRoot)
	before, err := os.ReadFile(cfgPath)
	if err != nil {
		return "", 0, nil, fmt.Errorf("read config for rollback: %w", err)
	}
	id, seq, err := s.NextID(ctx)
	if err != nil {
		return "", 0, nil, err
	}
	rollback := func() error {
		return os.WriteFile(cfgPath, before, 0o644)
	}
	return id, seq, rollback, nil
}

func acceptanceCriteriaFromSpec(ac []string) []ticket.AcceptanceCriterion {
	if len(ac) == 0 {
		return nil
	}
	out := make([]ticket.AcceptanceCriterion, 0, len(ac))
	for _, entry := range ac {
		out = append(out, ticket.AcceptanceCriterion{Description: entry})
	}
	return out
}

func readTicketApplySpec(cmd *cobra.Command, path string) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("reading --spec from stdin: %w", err)
		}
		return data, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading --spec file: %w", err)
	}
	return data, nil
}

func parseTicketPresence(raw []byte) (ticketApplyPresence, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var root map[string]any
	if err := dec.Decode(&root); err != nil {
		return ticketApplyPresence{}, err
	}
	ticketRaw, ok := root["ticket"]
	if !ok {
		return ticketApplyPresence{}, nil
	}
	ticketObj, ok := ticketRaw.(map[string]any)
	if !ok {
		return ticketApplyPresence{}, nil
	}
	_, id := ticketObj["id"]
	_, title := ticketObj["title"]
	_, desc := ticketObj["description"]
	_, priority := ticketObj["priority"]
	_, state := ticketObj["state"]
	_, labels := ticketObj["labels"]
	_, parent := ticketObj["parent"]
	_, blockedBy := ticketObj["blocked_by"]
	_, ac := ticketObj["ac"]

	return ticketApplyPresence{
		ID:          id,
		Title:       title,
		Description: desc,
		Priority:    priority,
		State:       state,
		Labels:      labels,
		Parent:      parent,
		BlockedBy:   blockedBy,
		AC:          ac,
	}, nil
}

func init() {
	ticketApplyCmd.Flags().StringVar(&ticketApplySpecPath, "spec", "", "spec file path (use - for stdin)")
	ticketCmd.AddCommand(ticketApplyCmd)
}
