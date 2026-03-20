package cmd

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/lifecycle"
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
	proofListKind   string
	proofListSince  string
	proofListActor  string
	proofListLimit  int
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
		t, err := s.GetTicket(context.Background(), args[0])
		if err != nil {
			return err
		}
		if t == nil {
			return fmt.Errorf("ticket %s not found", args[0])
		}
		now := time.Now().UTC().Truncate(time.Second)
		actor := detectActor()
		proofs := proof.NewRepositoryWithSourceRoot(ticketRepoRoot(repo), repo)
		rec, err := proofs.Add(context.Background(), proof.AddInput{
			TicketID:   t.ID,
			SourcePath: proofFile,
			ProofTitle: proofTitle,
			Note:       proofNote,
			AddedAt:    now.Format(time.RFC3339),
			CapturedAt: proofCapturedAt,
			Actor:      actor,
		})
		if err != nil {
			return err
		}
		emitProofMutationEvent(cmd, "add", "proof.add", t.ID, rec, actor)

		if format == "json" {
			printJSON(cmd, map[string]any{
				"ticket_id": t.ID,
				"proof":     rec,
			})
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Added proof %s to %s.\n", rec.ID, t.ID)
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
		defer func() {
			proofListKind = ""
			proofListSince = ""
			proofListActor = ""
			proofListLimit = 0
		}()

		s := local.New(repo)
		recs, err := s.ListProofs(context.Background(), args[0])
		if err != nil {
			return err
		}
		recs, err = filterAndSortProofs(recs, proofListKind, proofListSince, proofListActor, proofListLimit)
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
		emitProofMutationEvent(cmd, "remove", "proof.remove", args[0], removed, detectActor())

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

var proofGCCmd = &cobra.Command{
	Use:          "gc",
	Short:        "Garbage-collect unreferenced proof blobs",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) (runErr error) {
		defer func() {
			runErr = renderProofMutationError(cmd, runErr)
		}()

		s := local.New(repo)
		summary, err := s.GCProofBlobs(context.Background())
		if err != nil {
			return err
		}

		if format == "json" {
			printJSON(cmd, map[string]any{
				"gc": summary,
			})
			return nil
		}

		fmt.Fprintf(
			cmd.OutOrStdout(),
			"Proof GC complete: scanned=%d retained=%d removed=%d\n",
			summary.Scanned,
			summary.Retained,
			summary.Removed,
		)
		return nil
	},
}

func filterAndSortProofs(recs []proof.Record, kind string, since string, actor string, limit int) ([]proof.Record, error) {
	var sinceAt time.Time
	if strings.TrimSpace(since) != "" {
		parsed, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return nil, &proof.FieldError{
				ErrorCode:    "invalid_timestamp",
				Field:        "since",
				Retryable:    false,
				SuggestedFix: "use RFC3339 timestamp like 2026-03-16T18:00:00Z",
				Message:      "since must be RFC3339",
			}
		}
		sinceAt = parsed
	}

	normalizedKind := strings.ToLower(strings.TrimSpace(kind))
	if normalizedKind != "" && normalizedKind != "image" && normalizedKind != "screenshot" {
		return nil, &proof.FieldError{
			ErrorCode:    "invalid_field",
			Field:        "kind",
			Retryable:    false,
			SuggestedFix: "use --kind image or --kind screenshot",
			Message:      "kind must be one of: image, screenshot",
		}
	}
	if limit < 0 {
		return nil, &proof.FieldError{
			ErrorCode:    "invalid_field",
			Field:        "limit",
			Retryable:    false,
			SuggestedFix: "use a positive integer for --limit",
			Message:      "limit must be >= 0",
		}
	}

	filtered := make([]proof.Record, 0, len(recs))
	for _, rec := range recs {
		if normalizedKind != "" {
			k := proofKind(rec)
			if normalizedKind == "screenshot" && k != "image" {
				continue
			}
			if normalizedKind == "image" && k != "image" {
				continue
			}
		}
		if !sinceAt.IsZero() && rec.AddedAt.Before(sinceAt) {
			continue
		}
		if strings.TrimSpace(actor) != "" && rec.Actor != actor {
			continue
		}
		filtered = append(filtered, rec)
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].AddedAt.Equal(filtered[j].AddedAt) {
			return filtered[i].ID < filtered[j].ID
		}
		return filtered[i].AddedAt.After(filtered[j].AddedAt)
	})
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

func proofKind(rec proof.Record) string {
	if strings.HasPrefix(strings.ToLower(rec.File.MIMEType), "image/") {
		return "image"
	}
	return "other"
}

func emitProofMutationEvent(cmd *cobra.Command, action string, commandName string, ticketID string, rec *proof.Record, actor string) {
	if rec == nil {
		return
	}
	err := lifecycle.Append(repo, lifecycle.Event{
		Version:   lifecycle.SchemaVersionV1,
		Type:      lifecycle.EventProofMutation,
		EmittedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{
			"command":     commandName,
			"ticket_id":   ticketID,
			"proof_id":    rec.ID,
			"blob_sha256": rec.File.SHA256,
			"actor":       actor,
			"action":      action,
		},
	})
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "docket: warning: proof lifecycle event emit failed: %v\n", err)
	}
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
	proofListCmd.Flags().StringVar(&proofListKind, "kind", "", "filter proofs by kind (image|screenshot)")
	proofListCmd.Flags().StringVar(&proofListSince, "since", "", "filter proofs by added_at >= RFC3339 timestamp")
	proofListCmd.Flags().StringVar(&proofListActor, "actor", "", "filter proofs by actor")
	proofListCmd.Flags().IntVar(&proofListLimit, "limit", 0, "limit number of returned proofs (0 = no limit)")

	proofCmd.AddCommand(proofAddCmd)
	proofCmd.AddCommand(proofListCmd)
	proofCmd.AddCommand(proofRemoveCmd)
	proofCmd.AddCommand(proofGCCmd)
	rootCmd.AddCommand(proofCmd)
}
