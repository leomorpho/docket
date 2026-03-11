package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	updateState        string
	updatePriority     int
	updateTitle        string
	updateLabels       string
	updateAddLabels    []string
	updateRemoveLabels []string
	updateBlockedBy    []string
	updateUnblock      []string
	updateDesc         string
	updateHandoff      string
)

var updateCmd = &cobra.Command{
	Use:   "update <TKT-NNN>",
	Short: "Update ticket fields",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		s := local.New(repo)
		ctx := context.Background()

		t, err := s.GetTicket(ctx, id)
		if err != nil {
			return fmt.Errorf("getting ticket: %w", err)
		}
		if t == nil {
			return fmt.Errorf("ticket %s not found", id)
		}

		var updatedFields []string

		// 1. Title
		if cmd.Flags().Changed("title") {
			t.Title = updateTitle
			updatedFields = append(updatedFields, "title")
		}

		// 2. State
		if cmd.Flags().Changed("state") && updateState != "" {
			cfg, err := ticket.LoadConfig(repo)
			if err != nil {
				return err
			}
			if !cfg.IsValidState(updateState) {
				return fmt.Errorf("%q is not a valid state", updateState)
			}
			newState := ticket.State(updateState)
			if err := ticket.ValidateTransition(cfg, t.State, newState); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated %s: state %s → %s\n", t.ID, t.State, newState)
			t.State = newState
			updatedFields = append(updatedFields, "state")
		}

		// 3. Priority
		if cmd.Flags().Changed("priority") {
			t.Priority = updatePriority
			updatedFields = append(updatedFields, "priority")
		}

		// 4. Description
		if cmd.Flags().Changed("desc") {
			value, err := readUpdateValue(updateDesc)
			if err != nil {
				return err
			}
			t.Description = value
			updatedFields = append(updatedFields, "description")
		}

		// 5. Handoff
		if cmd.Flags().Changed("handoff") {
			value, err := readUpdateValue(updateHandoff)
			if err != nil {
				return err
			}
			t.Handoff = value
			updatedFields = append(updatedFields, "handoff")
		}

		// 6. Labels (replacement)
		if cmd.Flags().Changed("labels") {
			var labelList []string
			if updateLabels != "" {
				for _, l := range strings.Split(updateLabels, ",") {
					labelList = append(labelList, strings.TrimSpace(l))
				}
			}
			t.Labels = labelList
			updatedFields = append(updatedFields, "labels")
		}

		// 7. Add labels
		if len(updateAddLabels) > 0 {
			for _, l := range updateAddLabels {
				found := false
				for _, existing := range t.Labels {
					if existing == l {
						found = true
						break
					}
				}
				if !found {
					t.Labels = append(t.Labels, l)
				}
			}
			updatedFields = append(updatedFields, "add-label")
		}

		// 8. Remove labels
		if len(updateRemoveLabels) > 0 {
			var newLabels []string
			for _, existing := range t.Labels {
				remove := false
				for _, toRemove := range updateRemoveLabels {
					if existing == toRemove {
						remove = true
						break
					}
				}
				if !remove {
					newLabels = append(newLabels, existing)
				}
			}
			t.Labels = newLabels
			updatedFields = append(updatedFields, "remove-label")
		}

		// 9. Blocked by (add)
		if len(updateBlockedBy) > 0 {
			for _, b := range updateBlockedBy {
				found := false
				for _, existing := range t.BlockedBy {
					if existing == b {
						found = true
						break
					}
				}
				if !found {
					t.BlockedBy = append(t.BlockedBy, b)
				}
			}
			updatedFields = append(updatedFields, "blocked-by")
		}

		// 10. Unblock (remove)
		if len(updateUnblock) > 0 {
			var newBlockedBy []string
			for _, existing := range t.BlockedBy {
				remove := false
				for _, toRemove := range updateUnblock {
					if existing == toRemove {
						remove = true
						break
					}
				}
				if !remove {
					newBlockedBy = append(newBlockedBy, existing)
				}
			}
			t.BlockedBy = newBlockedBy
			updatedFields = append(updatedFields, "unblock")
		}

		if len(updatedFields) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No fields updated.")
			return nil
		}

		t.UpdatedAt = time.Now().UTC().Truncate(time.Second)

		if err := s.UpdateTicket(ctx, t); err != nil {
			return fmt.Errorf("updating ticket: %w", err)
		}

		if format == "json" {
			res := map[string]interface{}{
				"id":             t.ID,
				"updated_fields": updatedFields,
			}
			if cmd.Flags().Changed("state") {
				res["state"] = t.State
			}
			if cmd.Flags().Changed("priority") {
				res["priority"] = t.Priority
			}
			printJSON(cmd, res)
		} else if !cmd.Flags().Changed("state") {
			fmt.Fprintf(cmd.OutOrStdout(), "Updated %s: %s\n", t.ID, strings.Join(updatedFields, ", "))
		}

		// Reset global variables for test isolation
		updateState = ""
		updatePriority = 0
		updateTitle = ""
		updateLabels = ""
		updateAddLabels = nil
		updateRemoveLabels = nil
		updateBlockedBy = nil
		updateUnblock = nil
		updateDesc = ""
		updateHandoff = ""

		return nil
	},
}

func readUpdateValue(value string) (string, error) {
	if value != "-" {
		return value, nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("reading from stdin: %w", err)
	}
	return string(data), nil
}

func init() {
	updateCmd.Flags().StringVar(&updateState, "state", "", "new state")
	updateCmd.Flags().IntVar(&updatePriority, "priority", 0, "new priority")
	updateCmd.Flags().StringVar(&updateTitle, "title", "", "new title")
	updateCmd.Flags().StringVar(&updateHandoff, "handoff", "", "new handoff (use - for stdin)")
	updateCmd.Flags().StringVar(&updateLabels, "labels", "", "replace all labels (csv)")
	updateCmd.Flags().StringSliceVar(&updateAddLabels, "add-label", []string{}, "add one or more labels")
	updateCmd.Flags().StringSliceVar(&updateRemoveLabels, "remove-label", []string{}, "remove one or more labels")
	updateCmd.Flags().StringSliceVar(&updateBlockedBy, "blocked-by", []string{}, "add a blocker ticket ID")
	updateCmd.Flags().StringSliceVar(&updateUnblock, "unblock", []string{}, "remove a blocker ticket ID")
	updateCmd.Flags().StringVar(&updateDesc, "desc", "", "new description (use - for stdin)")

	rootCmd.AddCommand(updateCmd)
}
