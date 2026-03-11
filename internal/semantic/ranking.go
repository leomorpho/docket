package semantic

import (
	"context"
	"sort"
	"strings"
	"unicode"

	"github.com/leoaudibert/docket/internal/ticket"
)

type FieldWeights struct {
	Title       float64
	Description float64
	AC          float64
	Handoff     float64
}

func DefaultFieldWeights() FieldWeights {
	return FieldWeights{
		Title:       3.0,
		Description: 1.5,
		AC:          2.0,
		Handoff:     1.25,
	}
}

func (cfg Config) FieldWeights() FieldWeights {
	weights := DefaultFieldWeights()
	if cfg.TitleWeight > 0 {
		weights.Title = cfg.TitleWeight
	}
	if cfg.DescriptionWeight > 0 {
		weights.Description = cfg.DescriptionWeight
	}
	if cfg.ACWeight > 0 {
		weights.AC = cfg.ACWeight
	}
	if cfg.HandoffWeight > 0 {
		weights.Handoff = cfg.HandoffWeight
	}
	return weights
}

func (w FieldWeights) ForChunkType(chunkType ChunkType) float64 {
	switch chunkType {
	case ChunkTypeTitle:
		return w.Title
	case ChunkTypeDescription:
		return w.Description
	case ChunkTypeAC:
		return w.AC
	case ChunkTypeHandoff:
		return w.Handoff
	default:
		return 0
	}
}

type LexicalScore struct {
	TicketID string
	Score    float64
	Fields   map[ChunkType]float64
}

type VectorScore struct {
	TicketID string
	Score    float64
	Fields   map[ChunkType]float64
}

type CombinedScore struct {
	TicketID      string
	Score         float64
	LexicalScore  float64
	VectorScore   float64
	GraphScore    float64
	FieldScores   map[ChunkType]float64
	MatchedChunks []string
}

type LexicalScorer interface {
	ScoreRelated(source *ticket.Ticket, candidates []*ticket.Ticket, weights FieldWeights) []LexicalScore
}

type SimpleLexicalScorer struct{}

func (SimpleLexicalScorer) ScoreRelated(source *ticket.Ticket, candidates []*ticket.Ticket, weights FieldWeights) []LexicalScore {
	if source == nil {
		return nil
	}

	sourceFields := fieldTexts(source)
	sourceTokens := map[ChunkType]map[string]struct{}{}
	maxScore := 0.0
	for chunkType, text := range sourceFields {
		tokens := tokenize(text)
		if len(tokens) == 0 {
			continue
		}
		sourceTokens[chunkType] = tokens
		maxScore += weights.ForChunkType(chunkType)
	}
	if maxScore == 0 {
		return nil
	}

	scores := make([]LexicalScore, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == nil || candidate.ID == "" || candidate.ID == source.ID {
			continue
		}

		fields := map[ChunkType]float64{}
		total := 0.0
		for chunkType, queryTokens := range sourceTokens {
			candidateTokens := tokenize(fieldTexts(candidate)[chunkType])
			if len(candidateTokens) == 0 {
				continue
			}
			fieldScore := overlapScore(queryTokens, candidateTokens) * weights.ForChunkType(chunkType)
			if fieldScore == 0 {
				continue
			}
			fields[chunkType] = fieldScore
			total += fieldScore
		}
		if total == 0 {
			continue
		}

		scores = append(scores, LexicalScore{
			TicketID: candidate.ID,
			Score:    clamp01(total / maxScore),
			Fields:   fields,
		})
	}

	sort.Slice(scores, func(i, j int) bool {
		if scores[i].Score == scores[j].Score {
			return scores[i].TicketID < scores[j].TicketID
		}
		return scores[i].Score > scores[j].Score
	})
	return scores
}

type VectorScorer struct {
	Provider Provider
	Store    *VectorStore
}

func (s VectorScorer) ScoreRelated(ctx context.Context, source *ticket.Ticket, cfg Config, limit int) ([]VectorScore, error) {
	if source == nil {
		return nil, nil
	}
	if s.Provider == nil {
		return nil, nil
	}
	if s.Store == nil {
		return nil, nil
	}

	chunks := ChunkTicket(source)
	if len(chunks) == 0 {
		return nil, nil
	}

	inputs := make([]Input, 0, len(chunks))
	chunkByID := make(map[string]Chunk, len(chunks))
	queryTypes := map[ChunkType]struct{}{}
	for _, chunk := range chunks {
		inputs = append(inputs, Input{
			ChunkID:  chunk.ID,
			TicketID: chunk.TicketID,
			Field:    string(chunk.Type),
			Text:     chunk.Text,
		})
		chunkByID[chunk.ID] = chunk
		queryTypes[chunk.Type] = struct{}{}
	}

	resp, err := s.Provider.Embed(ctx, EmbedRequest{Model: cfg.Model, Inputs: inputs})
	if err != nil {
		return nil, err
	}

	weights := cfg.FieldWeights()
	maxScore := 0.0
	for chunkType := range queryTypes {
		maxScore += weights.ForChunkType(chunkType)
	}
	if maxScore == 0 {
		return nil, nil
	}

	queryLimit := limit * 4
	if queryLimit < 8 {
		queryLimit = 8
	}
	aggregated := map[string]*VectorScore{}
	for _, embedded := range resp.Results {
		chunk, ok := chunkByID[embedded.ChunkID]
		if !ok {
			continue
		}
		results, err := s.Store.Query(ctx, float64To32(embedded.Vector), queryLimit)
		if err != nil {
			return nil, err
		}
		for _, result := range results {
			if result.TicketID == "" || result.TicketID == source.ID {
				continue
			}
			entry := aggregated[result.TicketID]
			if entry == nil {
				entry = &VectorScore{
					TicketID: result.TicketID,
					Fields:   map[ChunkType]float64{},
				}
				aggregated[result.TicketID] = entry
			}
			chunkType := result.Type
			if chunkType == "" {
				chunkType = chunk.Type
			}
			weighted := clamp01(float64(result.Similarity)) * weights.ForChunkType(chunkType)
			if weighted <= entry.Fields[chunkType] {
				continue
			}
			entry.Fields[chunkType] = weighted
		}
	}

	scores := make([]VectorScore, 0, len(aggregated))
	for _, entry := range aggregated {
		total := 0.0
		for _, fieldScore := range entry.Fields {
			total += fieldScore
		}
		entry.Score = clamp01(total / maxScore)
		scores = append(scores, *entry)
	}

	sort.Slice(scores, func(i, j int) bool {
		if scores[i].Score == scores[j].Score {
			return scores[i].TicketID < scores[j].TicketID
		}
		return scores[i].Score > scores[j].Score
	})
	if limit > 0 && len(scores) > limit {
		scores = scores[:limit]
	}
	return scores, nil
}

func CombineScores(cfg Config, lexical []LexicalScore, vector []VectorScore) []CombinedScore {
	lexicalWeight := cfg.LexicalWeight
	vectorWeight := cfg.VectorWeight
	weightSum := lexicalWeight + vectorWeight
	if weightSum <= 0 {
		lexicalWeight = 0.5
		vectorWeight = 0.5
		weightSum = 1
	}

	combined := map[string]*CombinedScore{}
	for _, score := range lexical {
		entry := ensureCombined(combined, score.TicketID)
		entry.LexicalScore = score.Score
		mergeFieldScores(entry.FieldScores, score.Fields)
	}
	for _, score := range vector {
		entry := ensureCombined(combined, score.TicketID)
		entry.VectorScore = score.Score
		mergeFieldScores(entry.FieldScores, score.Fields)
	}

	out := make([]CombinedScore, 0, len(combined))
	for _, entry := range combined {
		entry.Score = clamp01((entry.LexicalScore*lexicalWeight + entry.VectorScore*vectorWeight) / weightSum)
		out = append(out, *entry)
	}
	sortCombinedScores(out)
	return out
}

type GraphEdgeKind string

const (
	GraphEdgeBlockedBy       GraphEdgeKind = "blocked_by"
	GraphEdgeBlocks          GraphEdgeKind = "blocks"
	GraphEdgeLinkedCommit    GraphEdgeKind = "linked_commit"
	GraphEdgeSessionAdjacent GraphEdgeKind = "session_adjacent"
)

type GraphEdge struct {
	FromTicketID string
	ToTicketID   string
	Kind         GraphEdgeKind
}

type GraphInputs struct {
	SourceTicketID string
	Edges          []GraphEdge
}

func BuildGraphInputs(source *ticket.Ticket, candidates []*ticket.Ticket) GraphInputs {
	if source == nil {
		return GraphInputs{}
	}
	inputs := GraphInputs{SourceTicketID: source.ID}

	sourceBlockedBy := makeStringSet(source.BlockedBy)
	sourceBlocks := makeStringSet(source.Blocks)
	sourceCommits := makeStringSet(source.LinkedCommits)
	for _, candidate := range candidates {
		if candidate == nil || candidate.ID == "" || candidate.ID == source.ID {
			continue
		}
		if _, ok := sourceBlockedBy[candidate.ID]; ok {
			inputs.Edges = append(inputs.Edges, GraphEdge{FromTicketID: source.ID, ToTicketID: candidate.ID, Kind: GraphEdgeBlockedBy})
		}
		if _, ok := sourceBlocks[candidate.ID]; ok {
			inputs.Edges = append(inputs.Edges, GraphEdge{FromTicketID: source.ID, ToTicketID: candidate.ID, Kind: GraphEdgeBlocks})
		}
		candidateBlockedBy := makeStringSet(candidate.BlockedBy)
		candidateBlocks := makeStringSet(candidate.Blocks)
		if _, ok := candidateBlockedBy[source.ID]; ok {
			inputs.Edges = append(inputs.Edges, GraphEdge{FromTicketID: source.ID, ToTicketID: candidate.ID, Kind: GraphEdgeBlocks})
		}
		if _, ok := candidateBlocks[source.ID]; ok {
			inputs.Edges = append(inputs.Edges, GraphEdge{FromTicketID: source.ID, ToTicketID: candidate.ID, Kind: GraphEdgeBlockedBy})
		}
		for _, commit := range candidate.LinkedCommits {
			if _, ok := sourceCommits[commit]; ok {
				inputs.Edges = append(inputs.Edges, GraphEdge{FromTicketID: source.ID, ToTicketID: candidate.ID, Kind: GraphEdgeLinkedCommit})
				break
			}
		}
	}
	return inputs
}

func CalculateGraphBoosts(inputs GraphInputs) map[string]float64 {
	boosts := map[string]float64{}
	for _, edge := range inputs.Edges {
		boosts[edge.ToTicketID] += graphEdgeWeight(edge.Kind)
	}
	return boosts
}

func ApplyGraphBoosts(scores []CombinedScore, boosts map[string]float64) []CombinedScore {
	if len(boosts) == 0 {
		sortCombinedScores(scores)
		return scores
	}
	for i := range scores {
		boost := boosts[scores[i].TicketID]
		scores[i].GraphScore = boost
		scores[i].Score = clamp01(scores[i].Score + boost)
	}
	sortCombinedScores(scores)
	return scores
}

func graphEdgeWeight(kind GraphEdgeKind) float64 {
	switch kind {
	case GraphEdgeBlockedBy, GraphEdgeBlocks:
		return 0.15
	case GraphEdgeLinkedCommit:
		return 0.05
	case GraphEdgeSessionAdjacent:
		return 0.03
	default:
		return 0
	}
}

func ensureCombined(scores map[string]*CombinedScore, ticketID string) *CombinedScore {
	entry := scores[ticketID]
	if entry == nil {
		entry = &CombinedScore{
			TicketID:    ticketID,
			FieldScores: map[ChunkType]float64{},
		}
		scores[ticketID] = entry
	}
	return entry
}

func mergeFieldScores(dst map[ChunkType]float64, src map[ChunkType]float64) {
	for chunkType, score := range src {
		if score > dst[chunkType] {
			dst[chunkType] = score
		}
	}
}

func sortCombinedScores(scores []CombinedScore) {
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].Score == scores[j].Score {
			return scores[i].TicketID < scores[j].TicketID
		}
		return scores[i].Score > scores[j].Score
	})
}

func fieldTexts(t *ticket.Ticket) map[ChunkType]string {
	if t == nil {
		return map[ChunkType]string{}
	}
	acItems := make([]string, 0, len(t.AC))
	for _, ac := range t.AC {
		acItems = append(acItems, ac.Description)
	}
	return map[ChunkType]string{
		ChunkTypeTitle:       t.Title,
		ChunkTypeDescription: t.Description,
		ChunkTypeAC:          strings.Join(acItems, "\n"),
		ChunkTypeHandoff:     t.Handoff,
	}
}

func tokenize(text string) map[string]struct{} {
	text = strings.ToLower(text)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	out := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		out[part] = struct{}{}
	}
	return out
}

func overlapScore(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	shared := 0
	for token := range a {
		if _, ok := b[token]; ok {
			shared++
		}
	}
	if shared == 0 {
		return 0
	}
	return (2 * float64(shared)) / float64(len(a)+len(b))
}

func makeStringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		out[value] = struct{}{}
	}
	return out
}

func clamp01(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}
