package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/leomorpho/docket/internal/adapters"
	"github.com/spf13/cobra"
)

var bootstrapAdapter string

type bootstrapStep struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type bootstrapResult struct {
	Adapter         adapters.Metadata `json:"adapter"`
	AdapterSource   string            `json:"adapter_source,omitempty"`
	AdapterWarning  string            `json:"adapter_warning,omitempty"`
	Steps           []bootstrapStep   `json:"steps"`
	NextInstruction string            `json:"next_instruction"`
}

type bootstrapDeps struct {
	resolve      func(repoRoot, requested string) adapters.Adapter
	resolveInfo  func(repoRoot, requested string) adapters.Resolution
	coreInstall  func(repoRoot string) (bool, error)
	runInstall   func(context.Context, adapters.Adapter, adapters.InstallInput) error
	runBootstrap func(context.Context, adapters.Adapter, adapters.BootstrapInput) error
}

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Run idempotent setup for hooks, managed files, and adapter bootstrap surfaces",
	RunE: func(cmd *cobra.Command, args []string) error {
		defer func() {
			bootstrapAdapter = ""
			if f := cmd.Flags().Lookup("adapter"); f != nil {
				f.Changed = false
			}
		}()

		gitDir := filepath.Join(repo, ".git")
		if stat, err := os.Stat(gitDir); err != nil || !stat.IsDir() {
			return fmt.Errorf("git repository not detected at %s", gitDir)
		}

		res, err := executeBootstrap(context.Background(), repo, bootstrapAdapter, bootstrapDeps{})
		if err != nil {
			return err
		}

		if format == "json" {
			printJSON(cmd, res)
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Bootstrap adapter: %s\n", res.Adapter.ID)
		if res.AdapterSource != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Adapter source: %s\n", res.AdapterSource)
		}
		if res.AdapterWarning != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Adapter warning: %s\n", res.AdapterWarning)
		}
		for _, step := range res.Steps {
			if step.Message == "" {
				fmt.Fprintf(cmd.OutOrStdout(), "- %s: %s\n", step.Name, step.Status)
				continue
			}
			fmt.Fprintf(cmd.OutOrStdout(), "- %s: %s (%s)\n", step.Name, step.Status, step.Message)
		}
		fmt.Fprintln(cmd.OutOrStdout(), res.NextInstruction)
		return nil
	},
}

func executeBootstrap(ctx context.Context, repoRoot, requestedAdapter string, deps bootstrapDeps) (bootstrapResult, error) {
	if deps.resolve == nil && deps.resolveInfo == nil {
		reg := adapters.DefaultRegistry()
		deps.resolveInfo = reg.ResolveWithInfo
	}
	if deps.coreInstall == nil {
		deps.coreInstall = runCoreInstallStep
	}
	if deps.runInstall == nil {
		deps.runInstall = adapters.RunInstall
	}
	if deps.runBootstrap == nil {
		deps.runBootstrap = adapters.RunBootstrap
	}

	res := bootstrapResult{
		NextInstruction: "Run `docket start` to pick up your next ticket.",
	}
	var adapter adapters.Adapter
	if deps.resolveInfo != nil {
		decision := deps.resolveInfo(repoRoot, requestedAdapter)
		adapter = decision.Adapter
		res.AdapterSource = decision.Source
		res.AdapterWarning = decision.Warning
	} else {
		adapter = deps.resolve(repoRoot, requestedAdapter)
	}
	res.Adapter = adapter.Metadata()

	changed, err := deps.coreInstall(repoRoot)
	if err != nil {
		return res, fmt.Errorf("core install step failed: %w", err)
	}
	if changed {
		res.Steps = append(res.Steps, bootstrapStep{Name: "core install", Status: "changed"})
	} else {
		res.Steps = append(res.Steps, bootstrapStep{Name: "core install", Status: "no-change"})
	}

	if err := deps.runInstall(ctx, adapter, adapters.InstallInput{RepoRoot: repoRoot}); err != nil {
		if errors.Is(err, adapters.ErrIncompleteAdapter) || errors.Is(err, adapters.ErrUnsupportedAdapter) {
			res.Steps = append(res.Steps, bootstrapStep{Name: "adapter install", Status: "no-change", Message: err.Error()})
		} else {
			return res, fmt.Errorf("adapter install step failed: %w", err)
		}
	} else {
		res.Steps = append(res.Steps, bootstrapStep{Name: "adapter install", Status: "changed"})
	}

	if err := deps.runBootstrap(ctx, adapter, adapters.BootstrapInput{RepoRoot: repoRoot}); err != nil {
		if errors.Is(err, adapters.ErrIncompleteAdapter) || errors.Is(err, adapters.ErrUnsupportedAdapter) {
			res.Steps = append(res.Steps, bootstrapStep{Name: "adapter bootstrap", Status: "no-change", Message: err.Error()})
		} else {
			return res, fmt.Errorf("adapter bootstrap step failed: %w", err)
		}
	} else {
		res.Steps = append(res.Steps, bootstrapStep{Name: "adapter bootstrap", Status: "changed"})
	}

	return res, nil
}

func runCoreInstallStep(repoRoot string) (bool, error) {
	hookChanged, err := writeHook(repoRoot)
	if err != nil {
		return false, err
	}
	claudeChanged, err := ensureClaudeManagedBlock(repoRoot)
	if err != nil {
		return false, err
	}
	if err := ensureLocalArtifactsGitignored(repoRoot); err != nil {
		return false, err
	}
	if err := writeInstallManifest(repoRoot); err != nil {
		return false, err
	}
	if err := validateStarterScaffoldLayout(repoRoot); err != nil {
		return false, err
	}
	return hookChanged || claudeChanged, nil
}

func init() {
	bootstrapCmd.Flags().StringVar(&bootstrapAdapter, "adapter", "", "adapter ID override (default auto-detect)")
	rootCmd.AddCommand(bootstrapCmd)
}
