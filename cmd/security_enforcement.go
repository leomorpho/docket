package cmd

import (
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

func warnSecurityEnforcementBypassed(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	rootCmd.ErrOrStderr().Write([]byte("warning: " + message + "\n"))
}

