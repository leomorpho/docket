package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/security"
	"github.com/spf13/cobra"
)

const autoSecureUnlockTTL = 10 * time.Minute

func ensureSecureSessionActive(repoRoot string) error {
	session := security.NewSessionManager(docketHome)
	if err := session.RequireActive(repoRoot); err != nil {
		if !errors.Is(err, security.ErrSecureModeInactive) {
			return err
		}
		password := strings.TrimSpace(os.Getenv("DOCKET_KEYSTORE_PASSWORD"))
		if password == "" {
			return err
		}
		if unlockErr := session.Unlock(repoRoot, password, autoSecureUnlockTTL); unlockErr != nil {
			return unlockErr
		}
		return session.RequireActive(repoRoot)
	}
	return nil
}

func requirePrivilegedSurface(cmd *cobra.Command, ticketID, action string, yes bool) error {
	if strings.Contains(strings.ToLower(action), "-> done") && isLLMActor() {
		return fmt.Errorf("transition to the configured completed state (default `done`) is human-only. If you are an LLM agent, stop at the configured review state (default `in-review`) instead; that is enough to unblock yourself and hand off for human verification")
	}
	if !securityEnforcementEnabled(repo) {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: privileged enforcement is disabled (security_enforcement=false); allowing %q without secure-mode authorization\n", action)
		return nil
	}
	if ticketID == "" {
		return fmt.Errorf("--ticket is required for privileged operations")
	}
	session := security.NewSessionManager(docketHome)
	if err := ensureSecureSessionActive(repo); err != nil {
		return err
	}
	if err := confirmPrivilegedPrompt(cmd, yes, ticketID, action); err != nil {
		return err
	}
	return session.RecordPrivilegedAction(repo, ticketID, action)
}

func isLLMActor() bool {
	if strings.TrimSpace(os.Getenv("DOCKET_AGENT_ID")) != "" {
		return true
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(detectActor())), "agent:")
}
