package cmd

import (
	"fmt"

	"github.com/leomorpho/docket/internal/security"
	"github.com/spf13/cobra"
)

func requirePrivilegedSurface(cmd *cobra.Command, ticketID, action string, yes bool) error {
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
