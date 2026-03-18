package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/leomorpho/docket/internal/semantic"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

type searchView struct {
	Query        string              `json:"query"`
	SemanticMode relatedSemanticMode `json:"semantic_mode"`
	SemanticUsed bool                `json:"semantic_used"`
	Warnings     []string            `json:"warnings,omitempty"`
	Results      []searchResultView  `json:"results"`
}

type searchResultView struct {
	TicketID     string  `json:"ticket_id"`
	Title        string  `json:"title"`
	State        string  `json:"state"`
	Score        float64 `json:"score"`
	LexicalScore float64 `json:"lexical_score"`
	VectorScore  float64 `json:"vector_score"`
}

var (
	searchSemantic string
	searchLimit    int
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search tickets by free-text query",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.TrimSpace(args[0])
		if query == "" {
			return fmt.Errorf("query must not be empty")
		}
		mode, err := parseRelatedSemanticMode(searchSemantic)
		if err != nil {
			return err
		}
		if searchLimit <= 0 {
			return fmt.Errorf("limit must be greater than zero")
		}

		ctx := context.Background()
		store := local.New(repo)
		candidates, err := semantic.EnumerateTickets(ctx, store)
		if err != nil {
			return fmt.Errorf("listing tickets: %w", err)
		}
		weights, cfg, err := loadRelatedConfig(repo)
		if err != nil {
			return err
		}

		lexicalScores := semanticLexicalScorer.ScoreQuery(query, candidates, weights)
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
				vectorScores, err = semanticQueryVectorScoreFn(ctx, query, cfg, provider, repo, searchLimit)
				if err != nil {
					if mode == relatedSemanticOn {
						return fmt.Errorf("semantic search failed: %w", err)
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
		combined = applySearchIDBoost(query, combined, candidates)
		if searchLimit > 0 && len(combined) > searchLimit {
			combined = combined[:searchLimit]
		}

		candidateByID := make(map[string]*ticket.Ticket, len(candidates))
		for _, candidate := range candidates {
			if candidate != nil {
				candidateByID[candidate.ID] = candidate
			}
		}

		view := searchView{
			Query:        query,
			SemanticMode: mode,
			SemanticUsed: semanticUsed,
			Warnings:     dedupeWarnings(warnings),
			Results:      make([]searchResultView, 0, len(combined)),
		}
		for _, score := range combined {
			candidate := candidateByID[score.TicketID]
			if candidate == nil {
				continue
			}
			view.Results = append(view.Results, searchResultView{
				TicketID:     candidate.ID,
				Title:        candidate.Title,
				State:        string(candidate.State),
				Score:        score.Score,
				LexicalScore: score.LexicalScore,
				VectorScore:  score.VectorScore,
			})
		}

		if format == "json" {
			printJSON(cmd, view)
			return nil
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Search: %q\n", query)
		fmt.Fprintf(out, "Semantic mode: %s (used=%t)\n", mode, semanticUsed)
		for _, warning := range view.Warnings {
			fmt.Fprintf(out, "Warning: %s\n", warning)
		}
		if len(view.Results) == 0 {
			fmt.Fprintln(out, "No matching tickets found.")
			return nil
		}
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSTATE\tSCORE\tLEX\tVEC\tTITLE")
		for _, result := range view.Results {
			fmt.Fprintf(w, "%s\t%s\t%.3f\t%.3f\t%.3f\t%s\n",
				result.TicketID, result.State, result.Score, result.LexicalScore, result.VectorScore, result.Title)
		}
		w.Flush()
		return nil
	},
}

func applySearchIDBoost(query string, scores []semantic.CombinedScore, candidates []*ticket.Ticket) []semantic.CombinedScore {
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	if normalizedQuery == "" {
		return scores
	}
	indexByTicketID := make(map[string]int, len(scores))
	for i := range scores {
		indexByTicketID[scores[i].TicketID] = i
	}
	for _, candidate := range candidates {
		if candidate == nil || candidate.ID == "" {
			continue
		}
		ticketID := strings.ToLower(candidate.ID)
		boost := 0.0
		switch {
		case ticketID == normalizedQuery:
			boost = 1.0
		case strings.Contains(ticketID, normalizedQuery):
			boost = 0.95
		default:
			continue
		}
		if idx, ok := indexByTicketID[candidate.ID]; ok {
			scores[idx].LexicalScore = maxScore(scores[idx].LexicalScore, boost)
			scores[idx].Score = maxScore(scores[idx].Score, boost)
			continue
		}
		scores = append(scores, semantic.CombinedScore{
			TicketID:     candidate.ID,
			Score:        boost,
			LexicalScore: boost,
		})
		indexByTicketID[candidate.ID] = len(scores) - 1
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].Score == scores[j].Score {
			return scores[i].TicketID < scores[j].TicketID
		}
		return scores[i].Score > scores[j].Score
	})
	return scores
}

func maxScore(current, candidate float64) float64 {
	if candidate > current {
		return candidate
	}
	return current
}

func init() {
	searchCmd.Flags().StringVar(&searchSemantic, "semantic", string(relatedSemanticAuto), "semantic mode: off, auto, or on")
	searchCmd.Flags().IntVar(&searchLimit, "limit", 10, "maximum number of matching tickets to return")
	rootCmd.AddCommand(searchCmd)
}
