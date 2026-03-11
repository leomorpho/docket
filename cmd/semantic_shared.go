package cmd

import (
	"context"

	"github.com/leoaudibert/docket/internal/semantic"
	"github.com/leoaudibert/docket/internal/ticket"
)

var (
	semanticProviderFactory                        = semantic.NewProvider
	semanticFreshnessFn                            = semantic.CheckFreshness
	semanticIncrementalFn                          = semantic.IncrementalRebuild
	semanticFullFn                                 = semantic.FullRebuild
	semanticOpenVectorStore                        = semantic.OpenVectorStore
	semanticLexicalScorer   semantic.LexicalScorer = semantic.SimpleLexicalScorer{}
	semanticVectorScoreFn                          = func(ctx context.Context, source *ticket.Ticket, cfg semantic.Config, provider semantic.Provider, repoRoot string, limit int) ([]semantic.VectorScore, error) {
		store, err := semanticOpenVectorStore(repoRoot)
		if err != nil {
			return nil, err
		}
		return semantic.VectorScorer{Provider: provider, Store: store}.ScoreRelated(ctx, source, cfg, limit)
	}
)

func loadSemanticConfig(repoRoot string) (semantic.Config, error) {
	cfg, err := ticket.LoadConfig(repoRoot)
	if err != nil {
		return semantic.Config{}, err
	}
	return semantic.ConfigFromTicket(cfg.Semantic), nil
}

func buildSemanticProvider(ctx context.Context, repoRoot string, cfg semantic.Config) (semantic.Provider, error) {
	_ = ctx
	return semanticProviderFactory(cfg, semantic.ProviderOptions{RepoRoot: repoRoot})
}
