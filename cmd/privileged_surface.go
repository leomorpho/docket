package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/leomorpho/docket/internal/security"
	"github.com/spf13/cobra"
)

func requirePrivilegedSurface(cmd *cobra.Command, ticketID, action string, yes bool) error {
	if strings.Contains(strings.ToLower(action), "-> done") && isLLMActor() {
		return fmt.Errorf("transition to done is human-only. If you are an LLM agent, stop at `in-review` instead; that is enough to unblock yourself and hand off for human verification")
	}
	if ticketID == "" {
		return fmt.Errorf("--ticket is required for privileged operations")
	}
	session := security.NewSessionManager(docketHome)
	if err := session.RequireActive(repo); err != nil {
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
