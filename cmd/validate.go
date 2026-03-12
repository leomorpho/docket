package cmd

import (
	"context"
	"fmt"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/spf13/cobra"
)

var (
	showWarns bool
	strict    bool
)

var validateCmd = &cobra.Command{
	Use:   "validate [TKT-NNN]",
	Short: "Validate ticket schema and dependencies",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := local.New(repo)
		ctx := context.Background()

		reconcileResults, _ := s.ReconcileTampering(ctx)
		reconcileByID := map[string]local.ReconcileResult{}
		for _, r := range reconcileResults {
			reconcileByID[r.ID] = r
		}

		if len(args) == 1 {
			return runValidateOne(cmd, s, args[0], reconcileByID)
		}
		return runValidateAll(cmd, s, reconcileResults)
	},
}

func runValidateOne(cmd *cobra.Command, s *local.Store, id string, reconcileByID map[string]local.ReconcileResult) error {
	errs, warns, err := s.ValidateFile(id)
	if err != nil {
		return fmt.Errorf("reading ticket: %w", err)
	}
	if rec, ok := reconcileByID[id]; ok && len(rec.Changes) > 0 {
		ch := rec.Changes[0]
		if rec.Accepted {
			warns = append(warns, store.ValidationError{
				Field:   "direct-edit." + ch.Field,
				Message: fmt.Sprintf("accepted schema-valid direct edit (%q -> %q). Next time use: %s", ch.Expected, ch.Actual, prescriptiveCommand(id, ch.Field, ch.Actual)),
			})
		} else if rec.Reverted {
			errs = append(errs, store.ValidationError{
				Field:   "direct-edit." + ch.Field,
				Message: fmt.Sprintf("rejected invalid direct edit (%q -> %q) and reverted file. Use: %s", ch.Expected, ch.Actual, prescriptiveCommand(id, ch.Field, ch.Actual)),
			})
		} else {
			errs = append(errs, store.ValidationError{
				Field:   "direct-edit." + ch.Field,
				Message: fmt.Sprintf("detected invalid direct edit (%q -> %q). Use: %s", ch.Expected, ch.Actual, prescriptiveCommand(id, ch.Field, ch.Actual)),
			})
		}
	}

	if format == "json" {
		printJSON(cmd, map[string]any{
			"valid":    len(errs) == 0 && (!strict || len(warns) == 0),
			"errors":   errs,
			"warnings": warns,
			"strict":   strict,
		})
		if len(errs) > 0 {
			return fmt.Errorf("validation failed for %s", id)
		}
		if strict && len(warns) > 0 {
			return fmt.Errorf("strict validation failed for %s: %d warning(s)", id, len(warns))
		}
		return nil
	}

	if len(errs) > 0 {
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

	fmt.Fprintf(cmd.OutOrStdout(), "✓ %s valid\n", id)
	if showWarns || strict {
		for _, w := range warns {
			fmt.Fprintf(cmd.OutOrStdout(), "  ! warning: %s: %s\n", w.Field, w.Message)
		}
	}
	if strict && len(warns) > 0 {
		return fmt.Errorf("strict validation failed for %s: %d warning(s)", id, len(warns))
	}
	return nil
}

func runValidateAll(cmd *cobra.Command, s *local.Store, reconcileResults []local.ReconcileResult) error {
	allErrs, allWarns, err := s.ValidateAll(context.Background())
	if err != nil {
		return fmt.Errorf("validating all tickets: %w", err)
	}

	for _, rec := range reconcileResults {
		if len(rec.Changes) == 0 {
			continue
		}
		ch := rec.Changes[0]
		if rec.Accepted {
			allWarns[rec.ID] = append(allWarns[rec.ID], store.ValidationError{
				Field:   "direct-edit." + ch.Field,
				Message: fmt.Sprintf("accepted schema-valid direct edit (%q -> %q). Next time use: %s", ch.Expected, ch.Actual, prescriptiveCommand(rec.ID, ch.Field, ch.Actual)),
			})
			continue
		}
		msg := fmt.Sprintf("detected invalid direct edit (%q -> %q). Use: %s", ch.Expected, ch.Actual, prescriptiveCommand(rec.ID, ch.Field, ch.Actual))
		if rec.Reverted {
			msg = fmt.Sprintf("rejected invalid direct edit (%q -> %q) and reverted file. Use: %s", ch.Expected, ch.Actual, prescriptiveCommand(rec.ID, ch.Field, ch.Actual))
		}
		allErrs[rec.ID] = append(allErrs[rec.ID], store.ValidationError{
			Field:   "direct-edit." + ch.Field,
			Message: msg,
		})
	}

	warnCount := 0
	for _, warns := range allWarns {
		warnCount += len(warns)
	}
	invalidCount := len(allErrs)

	if format == "json" {
		printJSON(cmd, map[string]any{
			"errors":   allErrs,
			"warnings": allWarns,
			"strict":   strict,
		})
		if invalidCount > 0 {
			return fmt.Errorf("validation failed for %d tickets", invalidCount)
		}
		if strict && warnCount > 0 {
			return fmt.Errorf("strict validation failed: %d warning(s)", warnCount)
		}
		return nil
	}

	if invalidCount > 0 {
		for id, errs := range allErrs {
			fmt.Fprintf(cmd.OutOrStdout(), "✗ %s invalid:\n", id)
			for _, e := range errs {
				fmt.Fprintf(cmd.OutOrStdout(), "  - %s: %s\n", e.Field, e.Message)
			}
		}
		if showWarns || strict {
			for id, warns := range allWarns {
				for _, w := range warns {
					fmt.Fprintf(cmd.OutOrStdout(), "  ! %s warning: %s: %s\n", id, w.Field, w.Message)
				}
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\nFound %d invalid tickets.\n", invalidCount)
		return fmt.Errorf("validation failed for %d tickets", invalidCount)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "All tickets valid.")
	if showWarns || strict {
		for id, warns := range allWarns {
			for _, w := range warns {
				fmt.Fprintf(cmd.OutOrStdout(), "  ! %s warning: %s: %s\n", id, w.Field, w.Message)
			}
		}
	}
	if strict && warnCount > 0 {
		return fmt.Errorf("strict validation failed: %d warning(s)", warnCount)
	}
	return nil
}

func prescriptiveCommand(id, field, value string) string {
	switch field {
	case "state":
		return fmt.Sprintf("docket update %s --state %s", id, value)
	case "priority":
		return fmt.Sprintf("docket update %s --priority %s", id, value)
	case "title":
		return fmt.Sprintf("docket update %s --title %q", id, value)
	case "parent":
		if value == "" {
			return fmt.Sprintf("docket update %s --parent none", id)
		}
		return fmt.Sprintf("docket update %s --parent %s", id, value)
	default:
		return fmt.Sprintf("docket show %s", id)
	}
}

func init() {
	validateCmd.Flags().BoolVarP(&showWarns, "warn", "w", false, "show warnings")
	validateCmd.Flags().BoolVar(&strict, "strict", false, "treat warnings as failures")
	rootCmd.AddCommand(validateCmd)
}
