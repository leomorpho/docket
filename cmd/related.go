package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/leoaudibert/docket/internal/semantic"
	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
	"github.com/spf13/cobra"
)

type relatedSemanticMode string

const (
	relatedSemanticOff  relatedSemanticMode = "off"
	relatedSemanticAuto relatedSemanticMode = "auto"
	relatedSemanticOn   relatedSemanticMode = "on"
)

type relatedView struct {
	SourceTicketID string              `json:"source_ticket_id"`
	SemanticMode   relatedSemanticMode `json:"semantic_mode"`
	SemanticUsed   bool                `json:"semantic_used"`
	Warnings       []string            `json:"warnings,omitempty"`
	Results        []relatedResultView `json:"results"`
}

type relatedResultView struct {
	TicketID     string  `json:"ticket_id"`
	Title        string  `json:"title"`
	State        string  `json:"state"`
	Score        float64 `json:"score"`
	LexicalScore float64 `json:"lexical_score"`
	VectorScore  float64 `json:"vector_score"`
	GraphScore   float64 `json:"graph_score"`
}

var (
	relatedSemantic string
	relatedLimit    int
)

var relatedCmd = &cobra.Command{
	Use:   "related <TKT-NNN>",
	Short: "Find related tickets for a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mode, err := parseRelatedSemanticMode(relatedSemantic)
		if err != nil {
			return err
		}
		if relatedLimit <= 0 {
			return fmt.Errorf("limit must be greater than zero")
		}

		ctx := context.Background()
		store := local.New(repo)
		source, err := store.GetTicket(ctx, args[0])
		if err != nil {
			return fmt.Errorf("getting ticket: %w", err)
		}
		if source == nil {
			return fmt.Errorf("ticket %s not found", args[0])
		}

		candidates, err := semantic.EnumerateTickets(ctx, store)
		if err != nil {
			return fmt.Errorf("listing tickets: %w", err)
		}
		weights, cfg, err := loadRelatedConfig(repo)
		if err != nil {
			return err
		}

		lexicalScores := semanticLexicalScorer.ScoreRelated(source, candidates, weights)
		vectorScores := []semantic.VectorScore{}
		warnings := []string{}
		semanticUsed := false
		if mode != relatedSemanticOff {
			provider, status, freshness, semanticWarnings, ready, err := semanticExecutionState(ctx, repo, cfg)
			if err != nil {
				return err
			}
			warnings = append(warnings, semanticWarnings...)
			if ready {
				vectorScores, err = semanticVectorScoreFn(ctx, source, cfg, provider, repo, relatedLimit)
				if err != nil {
					if mode == relatedSemanticOn {
						return fmt.Errorf("semantic related query failed: %w", err)
					}
					warnings = append(warnings, fmt.Sprintf("semantic fallback: %v", err))
				} else {
					semanticUsed = true
				}
			} else if mode == relatedSemanticOn {
				return fmt.Errorf("semantic mode unavailable: %s", semanticUnavailableReason(cfg, status, freshness))
			}
		}

		combined := semantic.CombineScores(cfg, lexicalScores, vectorScores)
		combined = semantic.ApplyGraphBoosts(combined, semantic.CalculateGraphBoosts(semantic.BuildGraphInputs(source, candidates)))
		if relatedLimit > 0 && len(combined) > relatedLimit {
			combined = combined[:relatedLimit]
		}

		view := relatedView{
			SourceTicketID: source.ID,
			SemanticMode:   mode,
			SemanticUsed:   semanticUsed,
			Warnings:       dedupeWarnings(warnings),
			Results:        make([]relatedResultView, 0, len(combined)),
		}
		candidateByID := map[string]*ticket.Ticket{}
		for _, candidate := range candidates {
			if candidate != nil {
				candidateByID[candidate.ID] = candidate
			}
		}
		for _, score := range combined {
			candidate := candidateByID[score.TicketID]
			if candidate == nil {
				continue
			}
			view.Results = append(view.Results, relatedResultView{
				TicketID:     candidate.ID,
				Title:        candidate.Title,
				State:        string(candidate.State),
				Score:        score.Score,
				LexicalScore: score.LexicalScore,
				VectorScore:  score.VectorScore,
				GraphScore:   score.GraphScore,
			})
		}

		if format == "json" {
			printJSON(cmd, view)
			return nil
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Related to %s\n", source.ID)
		fmt.Fprintf(out, "Semantic mode: %s (used=%t)\n", mode, semanticUsed)
		for _, warning := range view.Warnings {
			fmt.Fprintf(out, "Warning: %s\n", warning)
		}
		if len(view.Results) == 0 {
			fmt.Fprintln(out, "No related tickets found.")
			return nil
		}
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSTATE\tSCORE\tLEX\tVEC\tGRAPH\tTITLE")
		for _, result := range view.Results {
			fmt.Fprintf(w, "%s\t%s\t%.3f\t%.3f\t%.3f\t%.3f\t%s\n",
				result.TicketID, result.State, result.Score, result.LexicalScore, result.VectorScore, result.GraphScore, result.Title)
		}
		w.Flush()
		return nil
	},
}

func loadRelatedConfig(repoRoot string) (semantic.FieldWeights, semantic.Config, error) {
	cfg, err := loadSemanticConfig(repoRoot)
	if err != nil {
		return semantic.FieldWeights{}, semantic.Config{}, err
	}
	return cfg.FieldWeights(), cfg, nil
}

func parseRelatedSemanticMode(raw string) (relatedSemanticMode, error) {
	switch relatedSemanticMode(strings.TrimSpace(raw)) {
	case relatedSemanticOff:
		return relatedSemanticOff, nil
	case relatedSemanticAuto, "":
		return relatedSemanticAuto, nil
	case relatedSemanticOn:
		return relatedSemanticOn, nil
	default:
		return "", fmt.Errorf("invalid semantic mode %q (expected off, auto, or on)", raw)
	}
}

func semanticExecutionState(ctx context.Context, repoRoot string, cfg semantic.Config) (semantic.Provider, semantic.Status, semantic.Freshness, []string, bool, error) {
	if !cfg.Enabled {
		return nil, semantic.Status{}, semantic.Freshness{}, []string{"semantic mode is disabled in config"}, false, nil
	}

	provider, err := buildSemanticProvider(ctx, repoRoot, cfg)
	if err != nil {
		return nil, semantic.Status{}, semantic.Freshness{}, nil, false, err
	}
	status, err := provider.Status(ctx)
	if err != nil {
		return nil, semantic.Status{}, semantic.Freshness{}, nil, false, err
	}
	freshness, err := semanticFreshnessFn(ctx, repoRoot, cfg)
	if err != nil {
		return nil, status, semantic.Freshness{}, nil, false, err
	}

	warnings := semanticWarnings(status, freshness)
	return provider, status, freshness, warnings, semanticReady(status, freshness), nil
}

func semanticReady(status semantic.Status, freshness semantic.Freshness) bool {
	if !status.Available {
		return false
	}
	switch freshness.Status {
	case "", semantic.FreshnessFresh, semantic.FreshnessStale:
		return true
	default:
		return false
	}
}

func semanticUnavailableReason(cfg semantic.Config, status semantic.Status, freshness semantic.Freshness) string {
	if !cfg.Enabled {
		return "semantic mode is disabled in config"
	}
	if !status.Available {
		if status.Details != "" {
			return status.Details
		}
		return "provider is unavailable"
	}
	switch freshness.Status {
	case semantic.FreshnessMissing:
		return "semantic index is missing"
	case semantic.FreshnessVersionMismatch:
		return "semantic index version does not match current code"
	case semantic.FreshnessProviderMismatch:
		return "semantic index provider does not match current config"
	case semantic.FreshnessModelMismatch:
		return "semantic index model does not match current config"
	default:
		if freshness.Reason != "" {
			return freshness.Reason
		}
		return "semantic execution is unavailable"
	}
}

func semanticWarnings(status semantic.Status, freshness semantic.Freshness) []string {
	var warnings []string
	if !status.Available {
		message := "semantic provider is unavailable"
		if status.Details != "" {
			message += ": " + status.Details
		}
		warnings = append(warnings, message)
	}
	switch freshness.Status {
	case semantic.FreshnessStale:
		warnings = append(warnings, "semantic index is stale")
	case semantic.FreshnessMissing:
		warnings = append(warnings, "semantic index is missing")
	case semantic.FreshnessVersionMismatch:
		warnings = append(warnings, "semantic index version does not match current code")
	case semantic.FreshnessProviderMismatch:
		warnings = append(warnings, "semantic index provider does not match current config")
	case semantic.FreshnessModelMismatch:
		warnings = append(warnings, "semantic index model does not match current config")
	}
	return warnings
}

func dedupeWarnings(warnings []string) []string {
	if len(warnings) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		if warning == "" {
			continue
		}
		if _, ok := seen[warning]; ok {
			continue
		}
		seen[warning] = struct{}{}
		out = append(out, warning)
	}
	sort.Strings(out)
	return out
}

func init() {
	relatedCmd.Flags().StringVar(&relatedSemantic, "semantic", string(relatedSemanticAuto), "semantic mode: off, auto, or on")
	relatedCmd.Flags().IntVar(&relatedLimit, "limit", 5, "maximum number of related tickets to return")
	rootCmd.AddCommand(relatedCmd)
}
