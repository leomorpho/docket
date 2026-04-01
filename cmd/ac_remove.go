package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/spf13/cobra"
)

var (
	acRemoveStep string
	acRemoveYes  bool
)

var acRemoveCmd = &cobra.Command{
	Use:   "remove <TKT-NNN>",
	Short: "Remove an acceptance criterion",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer func() {
			resetACRemoveGlobals()
			resetACRemoveFlagChanges(cmd)
		}()
		id := args[0]
		if strings.TrimSpace(acRemoveStep) == "" {
			return fmt.Errorf("--step is required")
		}
		s := local.New(repo)
		t, err := s.GetTicket(context.Background(), id)
		if err != nil {
			return err
		}
		if t == nil {
			return fmt.Errorf("ticket %s not found", id)
		}
		idx, err := resolveACStep(t, acRemoveStep)
		if err != nil {
			return err
		}

		if !acRemoveYes {
			fmt.Fprintf(cmd.OutOrStdout(), "Removing AC %d on %s without interactive confirmation. Use --yes for compatibility.\n", idx+1, id)
		}

		t.AC = append(t.AC[:idx], t.AC[idx+1:]...)
		t.UpdatedAt = time.Now().UTC().Truncate(time.Second)
		if err := s.UpdateTicket(context.Background(), t); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Removed AC %d on %s.\n", idx+1, id)
		return nil
	},
}

func resetACRemoveGlobals() {
	acRemoveStep = ""
	acRemoveYes = false
}

func resetACRemoveFlagChanges(cmd *cobra.Command) {
	for _, name := range []string{"step", "yes"} {
		if f := cmd.Flags().Lookup(name); f != nil {
			f.Changed = false
		}
	}
}

func init() {
	acRemoveCmd.Flags().StringVar(&acRemoveStep, "step", "", "step index (1-based)")
	acRemoveCmd.Flags().BoolVar(&acRemoveYes, "yes", false, "skip confirmation prompt")
	acCmd.AddCommand(acRemoveCmd)
}
