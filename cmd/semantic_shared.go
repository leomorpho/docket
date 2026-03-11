package cmd

import (
	"context"

	"github.com/leoaudibert/docket/internal/semantic"
	"github.com/leoaudibert/docket/internal/ticket"
)

var (
	semanticProviderFactory = semantic.NewProvider
	semanticFreshnessFn     = semantic.CheckFreshness
	semanticIncrementalFn   = semantic.IncrementalRebuild
	semanticFullFn          = semantic.FullRebuild
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
