package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/leomorpho/docket/internal/adapters"
	"github.com/spf13/cobra"
)

var (
	installSkill  bool
	installCursor bool
	installVSCode bool
)

const installLongDesc = `Install docket-managed git hook and CLAUDE.md instructions.

Secure workflows rely on DOCKET_HOME to keep trusted artifacts outside the repository.
Set DOCKET_HOME to an absolute folder you control (for example: DOCKET_HOME=/home/alice/.docket-home) before running this command.
`

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install docket-managed git hook and CLAUDE.md instructions",
	Long:  installLongDesc,
	RunE: func(cmd *cobra.Command, args []string) error {
		if installSkill {
			return installAdapterSkills(cmd)
		}
		if installCursor {
			return installCursorRules(cmd)
		}
		if installVSCode {
			return installVSCodeSettings(cmd)
		}

		gitDir := filepath.Join(repo, ".git")
		if stat, err := os.Stat(gitDir); err != nil || !stat.IsDir() {
			return fmt.Errorf("git repository not detected at %s", gitDir)
		}

		hookChanged, err := writeHook(repo)
		if err != nil {
			return err
		}
		claudeChanged, err := ensureClaudeManagedBlock(repo)
		if err != nil {
			return err
		}
		if err := ensureLocalArtifactsGitignored(repo); err != nil {
			return err
		}
		if err := writeInstallManifest(repo); err != nil {
			return err
		}

		if format == "json" {
			printJSON(cmd, map[string]any{
				"hook_changed":   hookChanged,
				"claude_changed": claudeChanged,
				"manifest_path":  installManifestPath(repo),
			})
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Installed docket artifacts.\n")
		fmt.Fprintf(cmd.OutOrStdout(), "  hook: %s\n", preCommitHookPath(repo))
		fmt.Fprintf(cmd.OutOrStdout(), "  hook: %s\n", commitMsgHookPath(repo))
		fmt.Fprintf(cmd.OutOrStdout(), "  hook: %s\n", postMergeHookPath(repo))
		fmt.Fprintf(cmd.OutOrStdout(), "  manifest: %s\n", installManifestPath(repo))
		fmt.Fprintf(cmd.OutOrStdout(), "  secure storage (DOCKET_HOME): %s\n", docketHome)
		fmt.Fprintf(cmd.OutOrStdout(), "    Set DOCKET_HOME to a different writable directory if you prefer (example: DOCKET_HOME=%s)\n", filepath.Join(os.TempDir(), "docket-home"))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
	installCmd.Flags().BoolVar(&installSkill, "skill", false, "install Docket skills for Codex, Gemini CLI, and Claude Code")
	installCmd.Flags().BoolVar(&installCursor, "cursor", false, "install .cursorrules for Cursor")
	installCmd.Flags().BoolVar(&installVSCode, "vscode", false, "install .vscode/settings.json for VS Code")
}

func installCursorRules(cmd *cobra.Command) error {
	path := filepath.Join(repo, ".cursorrules")

	rules := `
# Docket Rules for Cursor Agents

- **CRITICAL: Do not edit .docket/tickets/*.md directly.** Always use the 'docket' MCP tools for all modifications.
- **Workflow:**
    1. **list** to find your assigned or next ticket.
    2. **show** to read the full specification and acceptance criteria.
    3. **update** the ticket to the repo's configured active work state. This will automatically claim the ticket and may create a dedicated git worktree for your changes.
    4. If a worktree was created, perform your work within that directory and stay on the managed Docket branch/worktree for the ticket.
    5. Once finished, ensure all acceptance criteria are met and tests pass.
    6. **update** the ticket to the repo's configured review state. This will automatically commit your changes, merge them back to the main branch, prune the linked worktree, and cleanup the claim.
    7. A human reviewer advances the ticket to the repo's configured completed state after verification.
- **Large Payloads:** If your content is > 1000 characters, write it to a temporary file and pass the path to the 'content_file' parameter in MCP calls.
`

	// Check if file exists to append or create
	existing, err := os.ReadFile(path)
	if err == nil {
		if strings.Contains(string(existing), "Docket Rules") {
			fmt.Fprintf(cmd.OutOrStdout(), ".cursorrules already contains Docket rules.\n")
			return nil
		}
		// Append
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := f.WriteString("\n" + rules); err != nil {
			return err
		}
	} else {
		// Create
		if err := os.WriteFile(path, []byte(rules), 0644); err != nil {
			return err
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Installed Docket rules to: %s\n", path)
	return nil
}

func installVSCodeSettings(cmd *cobra.Command) error {
	dir := filepath.Join(repo, ".vscode")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, "settings.json")

	// Standard MCP configuration for various extensions
	config := map[string]any{
		"mcp.servers": map[string]any{
			"docket": map[string]any{
				"command": "docket",
				"args":    []string{"serve", "--mcp", "--repo", repo},
			},
		},
	}

	// For simplicity in this implementation, we'll write/overwrite if not present
	// In a real scenario, we might want to merge with existing JSON
	existing, err := os.ReadFile(path)
	if err == nil {
		var m map[string]any
		if err := json.Unmarshal(existing, &m); err == nil {
			// Basic merge
			if m["mcp.servers"] == nil {
				m["mcp.servers"] = config["mcp.servers"]
			} else {
				servers := m["mcp.servers"].(map[string]any)
				servers["docket"] = config["mcp.servers"].(map[string]any)["docket"]
			}
			data, _ := json.MarshalIndent(m, "", "  ")
			if err := os.WriteFile(path, data, 0644); err != nil {
				return err
			}
		} else {
			// If JSON is invalid, just overwrite with our config
			data, _ := json.MarshalIndent(config, "", "  ")
			if err := os.WriteFile(path, data, 0644); err != nil {
				return err
			}
		}
	} else {
		data, _ := json.MarshalIndent(config, "", "  ")
		if err := os.WriteFile(path, data, 0644); err != nil {
			return err
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Installed VS Code settings to: %s\n", path)
	return nil
}

func installAdapterSkills(cmd *cobra.Command) error {
	installed, err := installAdapterSkillsForRepo(repo)
	if err != nil {
		return err
	}

	if format == "json" {
		printJSON(cmd, map[string]any{
			"installed_adapters": installed,
		})
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Installed Docket skills for adapters: %s\n", strings.Join(installed, ", "))
	fmt.Fprintf(cmd.OutOrStdout(), "  repo skill docs: %s, %s, %s\n",
		filepath.Join(repo, "AGENTS.md"),
		filepath.Join(repo, "CLAUDE.md"),
		filepath.Join(repo, "GEMINI.md"),
	)
	return nil
}

func installAdapterSkillsForRepo(repoRoot string) ([]string, error) {
	adapterIDs := []string{"codex", "gemini", "claude-code"}
	registry := adapters.DefaultRegistry()
	installed := make([]string, 0, len(adapterIDs))
	for _, adapterID := range adapterIDs {
		adapter := registry.Resolve(repoRoot, adapterID)
		if err := adapters.RunInstall(context.Background(), adapter, adapters.InstallInput{RepoRoot: repoRoot}); err != nil {
			return installed, fmt.Errorf("install skill artifacts for %s: %w", adapterID, err)
		}
		installed = append(installed, adapterID)
	}
	return installed, nil
}
