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
	if !yes {
		ok, err := security.ConfirmPrivilegedAction(cmd.InOrStdin(), cmd.OutOrStdout(), repo, ticketID, action)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("privileged action cancelled")
		}
	}
	return session.RecordPrivilegedAction(repo, ticketID, action)
}
