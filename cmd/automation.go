package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/leomorpho/docket/internal/security"
	"github.com/spf13/cobra"
)

var automationMode bool

func isAutomationMode() bool {
	if automationMode {
		return true
	}
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("DOCKET_AUTOMATION")))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func confirmPrivilegedPrompt(cmd *cobra.Command, yes bool, ticketID, action string) error {
	if yes {
		return nil
	}
	if isAutomationMode() {
		return fmt.Errorf("automation mode requires --yes for privileged operation %q", action)
	}
	ok, err := security.ConfirmPrivilegedAction(cmd.InOrStdin(), cmd.OutOrStdout(), repo, ticketID, action)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("privileged action cancelled")
	}
	return nil
}

