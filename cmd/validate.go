package cmd

import (
	"context"
	"fmt"

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/spf13/cobra"
)

var (
	showWarns bool
)

var validateCmd = &cobra.Command{
	Use:   "validate [TKT-NNN]",
	Short: "Validate ticket schema and dependencies",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := local.New(repo)
		ctx := context.Background()

		if len(args) == 1 {
			id := args[0]
			errs, warns, err := s.ValidateFile(id)
			if err != nil {
				return fmt.Errorf("reading ticket: %w", err)
			}

			if format == "json" {
				printJSON(cmd, map[string]interface{}{
					"valid":    len(errs) == 0,
					"errors":   errs,
					"warnings": warns,
				})
			} else {
				if len(errs) == 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "✓ %s valid\n", id)
					if showWarns && len(warns) > 0 {
						for _, w := range warns {
							fmt.Fprintf(cmd.OutOrStdout(), "  ! warning: %s: %s\n", w.Field, w.Message)
						}
					}
					return nil
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "✗ %s invalid:\n", id)
					for _, e := range errs {
						fmt.Fprintf(cmd.OutOrStdout(), "  - %s: %s\n", e.Field, e.Message)
					}
					if showWarns && len(warns) > 0 {
						for _, w := range warns {
							fmt.Fprintf(cmd.OutOrStdout(), "  ! warning: %s: %s\n", w.Field, w.Message)
						}
					}
					return fmt.Errorf("validation failed for %s", id)
				}
			}
		} else {
			allErrs, allWarns, err := s.ValidateAll(ctx)
			if err != nil {
				return fmt.Errorf("validating all tickets: %w", err)
			}

			if format == "json" {
				// This is a bit complex for JSON because we don't have a list of all IDs easily here
				// But we can just report what we found
				printJSON(cmd, map[string]interface{}{
					"errors":   allErrs,
					"warnings": allWarns,
				})
			} else {
				invalidCount := len(allErrs)
				// We'll need a way to list all IDs to show "✓" for valid ones.
				// For now, let's just show the errors if any.
				if invalidCount == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "All tickets valid.")
					return nil
				} else {
					for id, errs := range allErrs {
						fmt.Fprintf(cmd.OutOrStdout(), "✗ %s invalid:\n", id)
						for _, e := range errs {
							fmt.Fprintf(cmd.OutOrStdout(), "  - %s: %s\n", e.Field, e.Message)
						}
					}
					fmt.Fprintf(cmd.OutOrStdout(), "\nFound %d invalid tickets.\n", invalidCount)
					return fmt.Errorf("validation failed for %d tickets", invalidCount)
				}
			}
		}
		return nil
	},
}

func init() {
	validateCmd.Flags().BoolVarP(&showWarns, "warn", "w", false, "show warnings")
	rootCmd.AddCommand(validateCmd)
}
