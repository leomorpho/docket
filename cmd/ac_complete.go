package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	acCompleteStep     string
	acCompleteEvidence string
)

var acCompleteCmd = &cobra.Command{
	Use:   "complete <TKT-NNN>",
	Short: "Mark an acceptance criterion as complete",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		if strings.TrimSpace(acCompleteStep) == "" {
			return fmt.Errorf("--step is required")
		}
		if strings.TrimSpace(acCompleteEvidence) == "" {
			return fmt.Errorf("--evidence is required")
		}

		s := local.New(repo)
		t, err := s.GetTicket(context.Background(), id)
		if err != nil {
			return err
		}
		if t == nil {
			return fmt.Errorf("ticket %s not found", id)
		}

		idx, err := resolveACStep(t, acCompleteStep)
		if err != nil {
			return err
		}

		t.AC[idx].Done = true
		t.AC[idx].Evidence = strings.TrimSpace(acCompleteEvidence)
		t.UpdatedAt = time.Now().UTC().Truncate(time.Second)
		if err := s.UpdateTicket(context.Background(), t); err != nil {
			return err
		}
		_, _ = writeCheckpoint(repo, buildCheckpoint(repo, id, "AC completion checkpoint"))

		if format == "json" {
			printJSON(cmd, map[string]interface{}{"ticket_id": id, "step": idx + 1, "complete": true})
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Completed AC %d on %s.\n", idx+1, id)
		}
		return nil
	},
}

func resolveACStep(t *ticket.Ticket, step string) (int, error) {
	if n, err := strconv.Atoi(step); err == nil {
		if n <= 0 || n > len(t.AC) {
			return 0, fmt.Errorf("step %d is out of range", n)
		}
		return n - 1, nil
	}

	needle := strings.ToLower(strings.TrimSpace(step))
	for i, ac := range t.AC {
		if strings.Contains(strings.ToLower(ac.Description), needle) {
			return i, nil
		}
	}

	return 0, fmt.Errorf("no AC step matching %q", step)
}

func init() {
	acCompleteCmd.Flags().StringVar(&acCompleteStep, "step", "", "step index (1-based) or description substring")
	acCompleteCmd.Flags().StringVar(&acCompleteEvidence, "evidence", "", "evidence for completion")
	acCmd.AddCommand(acCompleteCmd)
}
