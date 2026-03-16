package cmd

import (
	"regexp"
	"sort"
	"strings"

	"github.com/leomorpho/docket/internal/learning"
	"github.com/leomorpho/docket/internal/ticket"
)

var learnTokenPattern = regexp.MustCompile(`[a-z0-9][a-z0-9_.-]*`)

type startLearnRule struct {
	Category string `json:"category"`
	Rule     string `json:"rule"`
	Source   string `json:"source,omitempty"`
}

type rankedLearnRule struct {
	startLearnRule
	score int
}

func buildLearnReplay(repoRoot string, t *ticket.Ticket, limit int) []startLearnRule {
	if t == nil || limit <= 0 {
		return nil
	}
	snapshot, err := learning.NewStore(repoRoot, nil).Load()
	if err != nil || len(snapshot.Entries) == 0 {
		return nil
	}
	queryTokens := ticketReplayTokens(t)
	if len(queryTokens) == 0 {
		return nil
	}

	ranked := make([]rankedLearnRule, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		score := replayScore(queryTokens, entry)
		if score <= 0 {
			continue
		}
		ranked = append(ranked, rankedLearnRule{
			startLearnRule: startLearnRule{
				Category: entry.Category,
				Rule:     entry.Rule,
				Source:   entry.Source,
			},
			score: score,
		})
	}
	if len(ranked) == 0 {
		return nil
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		if ranked[i].Category != ranked[j].Category {
			return ranked[i].Category < ranked[j].Category
		}
		if ranked[i].Rule != ranked[j].Rule {
			return ranked[i].Rule < ranked[j].Rule
		}
		return ranked[i].Source < ranked[j].Source
	})

	if limit > len(ranked) {
		limit = len(ranked)
	}
	out := make([]startLearnRule, 0, limit)
	for _, item := range ranked[:limit] {
		out = append(out, item.startLearnRule)
	}
	return out
}

func replayScore(query map[string]struct{}, entry learning.Entry) int {
	score := 0
	ruleTokens := tokenize(entry.Rule)
	for token := range ruleTokens {
		if _, ok := query[token]; ok {
			score++
		}
	}
	category := strings.ToLower(strings.TrimSpace(entry.Category))
	if category != "" {
		if _, ok := query[category]; ok {
			score += 3
		}
	}
	return score
}

func ticketReplayTokens(t *ticket.Ticket) map[string]struct{} {
	parts := []string{t.Title, t.Description}
	for _, label := range t.Labels {
		parts = append(parts, label)
	}
	for _, ac := range t.AC {
		parts = append(parts, ac.Description)
	}
	return tokenize(strings.Join(parts, " "))
}

func tokenize(text string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, token := range learnTokenPattern.FindAllString(strings.ToLower(text), -1) {
		out[token] = struct{}{}
	}
	return out
}
