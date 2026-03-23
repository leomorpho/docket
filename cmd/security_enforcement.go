package cmd

import (
	"fmt"
	"strings"

	"github.com/leomorpho/docket/internal/ticket"
)

func securityEnforcementEnabled(repoRoot string) bool {
	cfg, err := ticket.LoadConfig(repoRoot)
	if err != nil || cfg == nil {
		return false
	}
	return cfg.SecurityEnforcement
}

func securityEnforcementSurface(repoRoot string) (mode, note string) {
	cfg, err := ticket.LoadConfig(repoRoot)
	if err != nil {
		return securityEnforcementSurfaceFromConfig(nil)
	}
	return securityEnforcementSurfaceFromConfig(cfg)
}

func securityEnforcementSurfaceFromConfig(cfg *ticket.Config) (mode, note string) {
	if cfg != nil && cfg.SecurityEnforcement {
		return "enabled", "Privileged and terminal safeguards are enforced."
	}
	return "warning-only", "Privileged and terminal safeguards run in warning-only mode."
}

func warnSecurityEnforcementBypassed(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	_, _ = fmt.Fprintf(rootCmd.ErrOrStderr(), "warning: %s\n", message)
}
