package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	installSkill bool
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install docket-managed git hook and CLAUDE.md instructions",
	RunE: func(cmd *cobra.Command, args []string) error {
		if installSkill {
			return installGeminiSkill(cmd)
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
		fmt.Fprintf(cmd.OutOrStdout(), "  manifest: %s\n", installManifestPath(repo))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
	installCmd.Flags().BoolVar(&installSkill, "skill", false, "install Docket skill for Gemini CLI")
}

func installGeminiSkill(cmd *cobra.Command) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	skillDir := filepath.Join(home, ".gemini", "skills", "docket")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return fmt.Errorf("failed to create skill directory %s: %w", skillDir, err)
	}

	skillMD := `# Docket Skill

Specialized tool-use instructions for managing tickets via Docket.

## Tools

Docket provides a set of tools via its MCP server. Always prefer these tools over direct file modification.

- **list**: List tickets. Can filter by state (e.g., 'todo', 'in-progress', 'done').
- **create**: Create a new ticket. Requires 'title'. Optional: 'desc', 'state', 'priority'.
- **show**: Show details of a specific ticket. Requires 'id'.
- **update**: Update a ticket's state, title, or priority. Requires 'id'.
- **comment**: Add a comment to a ticket. Requires 'id' and 'body'.
- **check**: Run project-specific checks (e.g., tests, lint).

## Critical Directives

- **CRITICAL: Do not edit .docket/tickets/*.md directly.** Always use the 'docket' MCP tools for all modifications. Direct edits bypass validation and can lead to inconsistencies.
- **Workflow:**
    1. **list** to find your assigned or next ticket.
    2. **show** to read the full specification and acceptance criteria.
    3. **update** the ticket to 'in-progress'. This will automatically claim the ticket (TKT-142/143) and may create a dedicated git worktree for your changes.
    4. If a worktree was created, perform your work within that directory.
    5. Once finished, ensure all acceptance criteria are met and tests pass.
    6. **update** the ticket to 'done'. This will automatically commit your changes, merge them back to the main branch, and cleanup the worktree/claim.

## Performance & Context

- **Large Payloads (TKT-146):** If your content (description or handoff) is > 1000 characters, do not pass it directly through MCP. Instead, write the content to a temporary file (e.g., in /tmp/ or the project root) and pass the path to the 'content_file' parameter in 'create' or 'update' calls.
- Use **comment** to document progress or blockers for human review.
`

	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(skillMD), 0644); err != nil {
		return fmt.Errorf("failed to write SKILL.md to %s: %w", skillPath, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Installed Docket skill for Gemini CLI at: %s\n", skillDir)
	fmt.Fprintf(cmd.OutOrStdout(), "The agent will now use Docket MCP tools correctly and avoid direct file edits.\n")
	return nil
}
