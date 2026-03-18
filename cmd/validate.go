package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/spf13/cobra"
)

var (
	showWarns bool
	strict    bool
)

var validateCmd = &cobra.Command{
	Use:   "validate [TKT-NNN|path ...]",
	Short: "Validate ticket schema and dependencies",
	Args:  cobra.ArbitraryArgs,
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
		if len(args) > 1 {
			return runValidateMany(cmd, s, args, reconcileByID)
		}
		return runValidateAll(cmd, s, reconcileResults)
	},
}

func resetValidateGlobals() {
	showWarns = false
	strict = false
}

func runValidateOne(cmd *cobra.Command, s *local.Store, id string, reconcileByID map[string]local.ReconcileResult) error {
	displayID := normalizeTicketArg(id)
	errs, warns, err := validateTicketForCommand(s, displayID, reconcileByID)
	if err != nil {
		return err
	}

	if format == "json" {
		printJSON(cmd, map[string]any{
			"valid":    len(errs) == 0 && (!strict || len(warns) == 0),
			"errors":   errs,
			"warnings": warns,
			"strict":   strict,
		})
		if len(errs) > 0 {
			return fmt.Errorf("validation failed for %s", displayID)
		}
		if strict && len(warns) > 0 {
			return fmt.Errorf("strict validation failed for %s: %d warning(s)", displayID, len(warns))
		}
		return nil
	}

	if len(errs) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "✗ %s invalid:\n", displayID)
		for _, e := range errs {
			msg := e.Message
			if e.Field == "signature" {
				msg = fmt.Sprintf("write_hash is stale. Fix the schema errors above, then rerun `docket validate %s`.", displayID)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s: %s\n", e.Field, msg)
		}
		if showWarns && len(warns) > 0 {
			for _, w := range warns {
				fmt.Fprintf(cmd.OutOrStdout(), "  ! warning: %s: %s\n", w.Field, w.Message)
			}
		}
		return fmt.Errorf("validation failed for %s", displayID)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "✓ %s valid\n", displayID)
	if showWarns || strict {
		for _, w := range warns {
			fmt.Fprintf(cmd.OutOrStdout(), "  ! warning: %s: %s\n", w.Field, w.Message)
		}
	}
	if strict && len(warns) > 0 {
		return fmt.Errorf("strict validation failed for %s: %d warning(s)", displayID, len(warns))
	}
	return nil
}

func runValidateAll(cmd *cobra.Command, s *local.Store, reconcileResults []local.ReconcileResult) error {
	reconcileByID := map[string]local.ReconcileResult{}
	for _, rec := range reconcileResults {
		reconcileByID[rec.ID] = rec
	}
	ids, err := listTicketIDs(repo)
	if err != nil {
		return fmt.Errorf("validating all tickets: %w", err)
	}
	allErrs := make(map[string][]store.ValidationError)
	allWarns := make(map[string][]store.ValidationError)
	for _, id := range ids {
		errs, warns, err := validateTicketForCommand(s, id, reconcileByID)
		if err != nil {
			return fmt.Errorf("validating %s: %w", id, err)
		}
		if len(errs) > 0 {
			allErrs[id] = errs
		}
		if len(warns) > 0 {
			allWarns[id] = warns
		}
	}
	if cycleErr := s.DetectCycleValidationError(); cycleErr != nil {
		allErrs["global"] = append(allErrs["global"], *cycleErr)
	}
	return renderValidateSummary(cmd, allErrs, allWarns)
}

func runValidateMany(cmd *cobra.Command, s *local.Store, inputs []string, reconcileByID map[string]local.ReconcileResult) error {
	allErrs := make(map[string][]store.ValidationError)
	allWarns := make(map[string][]store.ValidationError)
	seen := map[string]struct{}{}
	for _, input := range inputs {
		id := normalizeTicketArg(input)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		errs, warns, err := validateTicketForCommand(s, id, reconcileByID)
		if err != nil {
			return fmt.Errorf("validating %s: %w", id, err)
		}
		if len(errs) > 0 {
			allErrs[id] = errs
		}
		if len(warns) > 0 {
			allWarns[id] = warns
		}
	}
	return renderValidateSummary(cmd, allErrs, allWarns)
}

func renderValidateSummary(cmd *cobra.Command, allErrs map[string][]store.ValidationError, allWarns map[string][]store.ValidationError) error {
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
				msg := e.Message
				if e.Field == "signature" {
					msg = fmt.Sprintf("write_hash is stale. Fix the schema errors above, then rerun `docket validate %s`.", id)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  - %s: %s\n", e.Field, msg)
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

func validateTicketForCommand(s *local.Store, id string, reconcileByID map[string]local.ReconcileResult) ([]store.ValidationError, []store.ValidationError, error) {
	errs, warns, err := s.ValidateFile(id)
	if err != nil {
		return nil, nil, fmt.Errorf("reading ticket: %w", err)
	}
	autoRepaired := false
	if hasSignatureError(errs) && countNonSignatureErrors(errs) == 0 {
		if repairedErr := autoRepairTicketSignature(s, id); repairedErr != nil {
			warns = append(warns, store.ValidationError{
				Field:   "signature",
				Message: fmt.Sprintf("ticket is schema-valid but write_hash refresh failed: %v", repairedErr),
			})
		} else {
			autoRepaired = true
			errs, warns, err = s.ValidateFile(id)
			if err != nil {
				return nil, nil, fmt.Errorf("reading ticket after auto-repair: %w", err)
			}
		}
	}
	if rec, ok := reconcileByID[id]; ok && len(rec.Changes) > 0 {
		for _, ch := range rec.Changes {
			if rec.Accepted {
				warns = append(warns, store.ValidationError{
					Field:   "direct-edit." + ch.Field,
					Message: fmt.Sprintf("accepted schema-valid direct edit (%q -> %q) and refreshed write_hash. Next time use: %s", ch.Expected, ch.Actual, prescriptiveCommand(id, ch.Field, ch.Actual)),
				})
				continue
			}
			msg := fmt.Sprintf("detected invalid direct edit (%q -> %q). Fix the markdown field and rerun `docket validate %s`. Equivalent CLI: %s", ch.Expected, ch.Actual, id, prescriptiveCommand(id, ch.Field, ch.Actual))
			if rec.Reverted {
				msg = fmt.Sprintf("rejected invalid direct edit (%q -> %q) and reverted file to the last legal value. If you want this change, reapply it in legal markdown form and rerun `docket validate %s`. Equivalent CLI: %s", ch.Expected, ch.Actual, id, prescriptiveCommand(id, ch.Field, ch.Actual))
			}
			errs = append(errs, store.ValidationError{
				Field:   "direct-edit." + ch.Field,
				Message: msg,
			})
		}
	}
	if autoRepaired && len(errs) == 0 && !hasDirectEditWarning(warns) {
		warns = append(warns, store.ValidationError{
			Field:   "direct-edit.signature",
			Message: fmt.Sprintf("accepted schema-valid direct markdown edit and refreshed write_hash. Next time rerun `docket validate %s` immediately after editing.", id),
		})
	}
	return errs, warns, nil
}

func hasSignatureError(errs []store.ValidationError) bool {
	for _, err := range errs {
		if err.Field == "signature" {
			return true
		}
	}
	return false
}

func countNonSignatureErrors(errs []store.ValidationError) int {
	count := 0
	for _, err := range errs {
		if err.Field != "signature" {
			count++
		}
	}
	return count
}

func hasDirectEditWarning(warns []store.ValidationError) bool {
	for _, warn := range warns {
		if strings.HasPrefix(warn.Field, "direct-edit.") {
			return true
		}
	}
	return false
}

func autoRepairTicketSignature(s *local.Store, id string) error {
	t, err := s.GetTicket(context.Background(), id)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("ticket %s not found", id)
	}
	return s.UpdateTicket(context.Background(), t)
}

func listTicketIDs(repoRoot string) ([]string, error) {
	ticketsDir := filepath.Join(repoRoot, ".docket", "tickets")
	entries, err := os.ReadDir(ticketsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || !strings.HasPrefix(entry.Name(), "TKT-") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(entry.Name(), ".md"))
	}
	sort.Strings(ids)
	return ids, nil
}

func normalizeTicketArg(raw string) string {
	if strings.HasSuffix(raw, ".md") {
		raw = strings.TrimSuffix(filepath.Base(raw), ".md")
	}
	return strings.TrimSpace(raw)
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
