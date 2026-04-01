package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/leomorpho/docket/internal/ticket"
)

// Retained only so older tests resetting this global still compile while the
// repo finishes shedding legacy secure-mode assumptions.
var docketHome string

func runtimeNamespaceRoot(repoRoot string) string {
	return defaultRuntimeNamespaceRoot(repoRoot)
}

func securityEnforcementSurface(repoRoot string) (mode, note string) {
	cfg, err := ticket.LoadConfig(repoRoot)
	if err == nil && cfg != nil && cfg.SecurityEnforcement {
		return "enabled", "Legacy secure-mode enforcement remains enabled only for backward-compatible managed-run checks."
	}
	return "warning-only", "Legacy secure-mode enforcement has been removed from the core runtime."
}

func warnSecurityEnforcementBypassed(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	_, _ = fmt.Fprintf(rootCmd.ErrOrStderr(), "warning: %s\n", message)
}

func defaultRuntimeNamespaceRoot(repoRoot string) string {
	root := ticketRepoRoot(repoRoot)
	return filepath.Join(root, ".docket", "local", "namespace")
}

func ensureRuntimeNamespaceRoot(repoRoot string) error {
	return os.MkdirAll(defaultRuntimeNamespaceRoot(repoRoot), 0o755)
}
