package cmd

import (
	"context"
	"fmt"
	"sort"

	"github.com/leomorpho/docket/internal/semantic"
	"github.com/spf13/cobra"
)

type semanticStatusView struct {
	Provider   string                  `json:"provider"`
	Model      string                  `json:"model"`
	Available  bool                    `json:"available"`
	Details    string                  `json:"details,omitempty"`
	Warnings   []string                `json:"warnings,omitempty"`
	CachePaths map[string]string       `json:"cache_paths"`
	Freshness  semantic.Freshness      `json:"freshness"`
	Index      semanticStatusIndexView `json:"index"`
}

type semanticStatusIndexView struct {
	ChunkCount  int    `json:"chunk_count"`
	TicketCount int    `json:"ticket_count"`
	Version     string `json:"version,omitempty"`
	LastUpdated string `json:"last_updated,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Model       string `json:"model,omitempty"`
}

var semanticStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show semantic provider and index status",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		cfg, err := loadSemanticConfig(repo)
		if err != nil {
			return err
		}

		provider, err := buildSemanticProvider(ctx, repo, cfg)
		if err != nil {
			return err
		}
		status, err := provider.Status(ctx)
		if err != nil {
			return err
		}

		metadata, err := semantic.LoadMetadata(repo)
		if err != nil {
			return err
		}
		freshness, err := semanticFreshnessFn(ctx, repo, cfg)
		if err != nil {
			return err
		}

		view := semanticStatusView{
			Provider:  status.Provider,
			Model:     status.Model,
			Available: status.Available,
			Details:   status.Details,
			Warnings:  semanticWarnings(status, freshness),
			CachePaths: map[string]string{
				"hf_home":                    cfg.HFHome,
				"sentence_transformers_home": cfg.SentenceTransformersHome,
				"uv_cache_dir":               cfg.UVCacheDir,
			},
			Freshness: freshness,
			Index: semanticStatusIndexView{
				ChunkCount:  len(metadata.Chunks),
				TicketCount: countMetadataTickets(metadata),
				Version:     metadata.Version,
				Provider:    metadata.Provider,
				Model:       metadata.Model,
			},
		}
		if !metadata.UpdatedAt.IsZero() {
			view.Index.LastUpdated = metadata.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z")
		}
		view.Warnings = dedupeWarnings(view.Warnings)

		if format == "json" {
			printJSON(cmd, view)
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Provider: %s\n", view.Provider)
		fmt.Fprintf(cmd.OutOrStdout(), "Model: %s\n", view.Model)
		fmt.Fprintf(cmd.OutOrStdout(), "Available: %t\n", view.Available)
		if view.Details != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Details: %s\n", view.Details)
		}
		for _, warning := range view.Warnings {
			fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", warning)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Cache paths:")
		keys := make([]string, 0, len(view.CachePaths))
		for key := range view.CachePaths {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", key, view.CachePaths[key])
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Freshness: %s\n", view.Freshness.Status)
		if view.Freshness.Reason != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Reason: %s\n", view.Freshness.Reason)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Index chunks: %d\n", view.Index.ChunkCount)
		fmt.Fprintf(cmd.OutOrStdout(), "Index tickets: %d\n", view.Index.TicketCount)
		if view.Index.Version != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Index version: %s\n", view.Index.Version)
		}
		if view.Index.Provider != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Indexed provider: %s\n", view.Index.Provider)
		}
		if view.Index.Model != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Indexed model: %s\n", view.Index.Model)
		}
		if view.Index.LastUpdated != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Last updated: %s\n", view.Index.LastUpdated)
		}
		return nil
	},
}

func countMetadataTickets(metadata *semantic.Metadata) int {
	unique := map[string]struct{}{}
	for _, chunk := range metadata.Chunks {
		unique[chunk.TicketID] = struct{}{}
	}
	return len(unique)
}

func init() {
	semanticCmd.AddCommand(semanticStatusCmd)
}
