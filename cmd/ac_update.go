package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/spf13/cobra"
)

var (
	acUpdateStep string
	acUpdateDesc string
	acUpdateRun  string
)

var acUpdateCmd = &cobra.Command{
	Use:   "update <TKT-NNN>",
	Short: "Update an acceptance criterion",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		if strings.TrimSpace(acUpdateStep) == "" {
			return fmt.Errorf("--step is required")
		}
		if strings.TrimSpace(acUpdateDesc) == "" && !cmd.Flags().Changed("run") {
			return fmt.Errorf("provide --desc and/or --run")
		}

		s := local.New(repo)
		t, err := s.GetTicket(context.Background(), id)
		if err != nil {
			return err
		}
		if t == nil {
			return fmt.Errorf("ticket %s not found", id)
		}

		idx, err := resolveACStep(t, acUpdateStep)
		if err != nil {
			return err
		}
		if strings.TrimSpace(acUpdateDesc) != "" {
			t.AC[idx].Description = strings.TrimSpace(acUpdateDesc)
		}
		if cmd.Flags().Changed("run") {
			t.AC[idx].Run = strings.TrimSpace(acUpdateRun)
		}
		t.UpdatedAt = time.Now().UTC().Truncate(time.Second)
		if err := s.UpdateTicket(context.Background(), t); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Updated AC %d on %s.\n", idx+1, id)
		return nil
	},
}

func init() {
	acUpdateCmd.Flags().StringVar(&acUpdateStep, "step", "", "step index (1-based)")
	acUpdateCmd.Flags().StringVar(&acUpdateDesc, "desc", "", "new acceptance criterion description")
	acUpdateCmd.Flags().StringVar(&acUpdateRun, "run", "", "new runnable command (set empty to clear with --run \"\")")
	acCmd.AddCommand(acUpdateCmd)
}
