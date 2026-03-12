package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/tui"
	"github.com/spf13/cobra"
)

var boardCmd = &cobra.Command{
	Use:   "board",
	Short: "Open the interactive kanban board",
	RunE: func(cmd *cobra.Command, args []string) error {
		s := local.New(repo)
		return runBoard(repo, s)
	},
}

// runBoard launches the bubbletea program.
func runBoard(repoRoot string, backend store.Backend) error {
	model := tui.NewBoardModel(repoRoot, backend, detectActor())
	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("running board: %w", err)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(boardCmd)
}
