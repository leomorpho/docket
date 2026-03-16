package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/leomorpho/docket/internal/proof"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/spf13/cobra"
)

var (
	proofFile       string
	proofTitle      string
	proofNote       string
	proofCapturedAt string
	proofID         string
)

var proofCmd = &cobra.Command{
	Use:   "proof",
	Short: "Manage screenshot proof metadata attached to tickets",
}

var proofAddCmd = &cobra.Command{
	Use:          "add <TKT-NNN>",
	Short:        "Attach a screenshot proof to a ticket",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) (runErr error) {
		defer func() {
			proofFile = ""
			proofTitle = ""
			proofNote = ""
			proofCapturedAt = ""
		}()
		defer func() {
			runErr = renderProofMutationError(cmd, runErr)
		}()

		s := local.New(repo)
		now := time.Now().UTC().Truncate(time.Second)
		rec, err := s.AddProof(context.Background(), proof.AddInput{
			TicketID:   args[0],
			SourcePath: proofFile,
			ProofTitle: proofTitle,
			Note:       proofNote,
			AddedAt:    now.Format(time.RFC3339),
			CapturedAt: proofCapturedAt,
			Actor:      detectActor(),
		})
		if err != nil {
			return err
		}

		if format == "json" {
			printJSON(cmd, map[string]any{
				"ticket_id": args[0],
				"proof":     rec,
			})
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Added proof %s to %s.\n", rec.ID, args[0])
		return nil
	},
}

var proofListCmd = &cobra.Command{
	Use:          "list <TKT-NNN>",
	Short:        "List screenshot proofs attached to a ticket",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) (runErr error) {
		defer func() {
			runErr = renderProofMutationError(cmd, runErr)
		}()

		s := local.New(repo)
		recs, err := s.ListProofs(context.Background(), args[0])
		if err != nil {
			return err
		}

		if format == "json" {
			printJSON(cmd, map[string]any{
				"ticket_id": args[0],
				"proofs":    recs,
			})
			return nil
		}

		if len(recs) == 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "No proofs on %s.\n", args[0])
			return nil
		}
		for _, rec := range recs {
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", rec.ID, rec.ProofTitle, rec.AddedAt.Format(time.RFC3339), rec.File.Path)
		}
		return nil
	},
}

var proofRemoveCmd = &cobra.Command{
	Use:          "remove <TKT-NNN>",
	Short:        "Remove a screenshot proof record from a ticket",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) (runErr error) {
		defer func() {
			proofID = ""
		}()
		defer func() {
			runErr = renderProofMutationError(cmd, runErr)
		}()

		s := local.New(repo)
		removed, err := s.RemoveProof(context.Background(), args[0], proofID)
		if err != nil {
			return err
		}

		if format == "json" {
			printJSON(cmd, map[string]any{
				"ticket_id": args[0],
				"removed":   removed,
			})
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Removed proof %s from %s.\n", removed.ID, args[0])
		return nil
	},
}

func renderProofMutationError(cmd *cobra.Command, err error) error {
	if err == nil {
		return nil
	}
	var fieldErr *proof.FieldError
	if errors.As(err, &fieldErr) {
		if format == "json" {
			printJSON(cmd, map[string]any{
				"error": "validation_failed",
				"error_envelope": mutationErrorEnvelope{
					ErrorCode:    fieldErr.ErrorCode,
					Field:        fieldErr.Field,
					Retryable:    fieldErr.Retryable,
					SuggestedFix: fieldErr.SuggestedFix,
					Message:      fieldErr.Message,
				},
			})
			return renderedMutationError{cause: err}
		}
		return err
	}
	return renderMutationError(cmd, err)
}

func init() {
	proofAddCmd.Flags().StringVar(&proofFile, "file", "", "relative path to proof image file")
	proofAddCmd.Flags().StringVar(&proofTitle, "proof-title", "", "short title for the screenshot proof")
	proofAddCmd.Flags().StringVar(&proofNote, "note", "", "narrative note describing why this proof was attached")
	proofAddCmd.Flags().StringVar(&proofCapturedAt, "captured-at", "", "RFC3339 timestamp for when screenshot was captured")

	proofRemoveCmd.Flags().StringVar(&proofID, "proof-id", "", "proof id to remove")

	proofCmd.AddCommand(proofAddCmd)
	proofCmd.AddCommand(proofListCmd)
	proofCmd.AddCommand(proofRemoveCmd)
	rootCmd.AddCommand(proofCmd)
}
