package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Launch the local Svelte UI dev server",
	Long:  "Starts the SvelteKit UI in web/ with DOCKET_DIR pointed at the current repository.",
	RunE: func(cmd *cobra.Command, args []string) error {
		webDir, err := resolveWebDir()
		if err != nil {
			return err
		}
		if _, err := exec.LookPath("pnpm"); err != nil {
			return fmt.Errorf("pnpm not found in PATH. Install pnpm, then retry `docket ui`")
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		proc := exec.Command("pnpm", "dev")
		proc.Dir = webDir
		proc.Env = append(os.Environ(), "DOCKET_DIR="+cwd)
		proc.Stdout = cmd.OutOrStdout()
		proc.Stderr = cmd.ErrOrStderr()
		proc.Stdin = os.Stdin
		return proc.Run()
	},
}

func resolveWebDir() (string, error) {
	if env := os.Getenv("DOCKET_UI_DIR"); env != "" {
		if hasWebPackage(env) {
			return env, nil
		}
		return "", fmt.Errorf("DOCKET_UI_DIR=%q does not contain web/package.json", env)
	}

	if exe, err := os.Executable(); err == nil {
		if p := walkForWeb(filepath.Dir(exe)); p != "" {
			return p, nil
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		if p := walkForWeb(cwd); p != "" {
			return p, nil
		}
	}

	return "", fmt.Errorf("web UI directory not found. Set DOCKET_UI_DIR to the web/ path")
}

func walkForWeb(start string) string {
	cur := start
	for i := 0; i < 12; i++ {
		candidate := filepath.Join(cur, "web")
		if hasWebPackage(candidate) {
			return candidate
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return ""
}

func hasWebPackage(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "package.json"))
	return err == nil && !info.IsDir()
}

func init() {
	rootCmd.AddCommand(uiCmd)
}
