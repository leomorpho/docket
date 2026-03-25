package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/leomorpho/docket/internal/adapters"
	"github.com/spf13/cobra"
)

func shouldAutoSyncSkills(cmd *cobra.Command, repoRoot string) bool {
	if cmd == nil {
		return false
	}
	switch cmd.Name() {
	case "help", "completion", "install", "bootstrap":
		return false
	}
	if len(cmd.Name()) >= 6 && cmd.Name()[:6] == "__hook" {
		return false
	}
	if repoRoot == "" {
		return false
	}
	stat, err := os.Stat(filepath.Join(repoRoot, ".git"))
	return err == nil && stat.IsDir()
}

func autoSyncSkills(repoRoot string) error {
	registry := adapters.DefaultRegistry()
	adapter := registry.Resolve(repoRoot, "")
	adapterID := adapter.Metadata().ID
	if adapterID == "unsupported" || adapterID == "unknown" || adapterID == "auto-detect" {
		return nil
	}
	if err := adapters.RunInstall(context.Background(), adapter, adapters.InstallInput{RepoRoot: repoRoot}); err != nil {
		return fmt.Errorf("auto skill sync for %s: %w", adapterID, err)
	}
	return nil
}
