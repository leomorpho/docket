package cmd

import (
	"fmt"
	"time"

	"github.com/leomorpho/docket/internal/security"
	"github.com/spf13/cobra"
)

var (
	securePassword string
	secureTTL      time.Duration
	secureTicket   string
	secureAction   string
	secureYes      bool
	secureSignerID string
)

var secureCmd = &cobra.Command{
	Use:   "secure",
	Short: "Manage secure-mode session for privileged operations",
}

var secureUnlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Unlock secure mode with a password and TTL",
	RunE: func(cmd *cobra.Command, args []string) error {
		if securePassword == "" {
			return fmt.Errorf("--password is required")
		}
		mgr := security.NewSessionManager(docketHome)
		if err := mgr.Unlock(repo, securePassword, secureTTL); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Secure mode active until %s\n", time.Now().UTC().Add(secureTTL).Format(time.RFC3339))
		return nil
	},
}

var secureLockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Lock secure mode immediately",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := security.NewSessionManager(docketHome)
		if err := mgr.Lock(); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Secure mode locked.")
		return nil
	},
}

var secureStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show secure-mode session status",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := security.NewSessionManager(docketHome)
		active, expiresAt, err := mgr.Status(repo)
		if err != nil {
			return err
		}
		if !active {
			fmt.Fprintln(cmd.OutOrStdout(), "Secure mode inactive.")
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Secure mode active (expires: %s)\n", expiresAt.Format(time.RFC3339))
		return nil
	},
}

var secureApproveCmd = &cobra.Command{
	Use:   "approve",
	Short: "Run a privileged approval mutation (requires secure mode + confirmation)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if secureTicket == "" {
			return fmt.Errorf("--ticket is required")
		}
		if secureAction == "" {
			return fmt.Errorf("--action is required")
		}

		mgr := security.NewSessionManager(docketHome)
		if err := mgr.RequireActive(repo); err != nil {
			return err
		}

		confirmed := secureYes
		if !confirmed {
			ok, err := security.ConfirmPrivilegedAction(cmd.InOrStdin(), cmd.OutOrStdout(), repo, secureTicket, secureAction)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("privileged action cancelled")
			}
		}

		if err := mgr.RecordPrivilegedAction(repo, secureTicket, secureAction); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Privileged action recorded for %s (%s)\n", secureTicket, secureAction)
		return nil
	},
}

var secureAnchorSetCmd = &cobra.Command{
	Use:   "set-anchor",
	Short: "Set repo trust anchor in DOCKET_HOME namespace (privileged)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if secureTicket == "" {
			return fmt.Errorf("--ticket is required")
		}
		if secureSignerID == "" {
			return fmt.Errorf("--signer-id is required")
		}

		mgr := security.NewSessionManager(docketHome)
		if err := mgr.RequireActive(repo); err != nil {
			return err
		}

		action := fmt.Sprintf("set trust anchor signer=%s", secureSignerID)
		confirmed := secureYes
		if !confirmed {
			ok, err := security.ConfirmPrivilegedAction(cmd.InOrStdin(), cmd.OutOrStdout(), repo, secureTicket, action)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("privileged action cancelled")
			}
		}

		ns := security.NewRepoNamespaceStore(docketHome)
		repoID, err := ns.SetTrustAnchor(repo, secureSignerID)
		if err != nil {
			return err
		}
		if err := mgr.RecordPrivilegedAction(repo, secureTicket, action); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Trust anchor set for repo %s (signer=%s)\n", repoID, secureSignerID)
		return nil
	},
}

func init() {
	secureUnlockCmd.Flags().StringVar(&securePassword, "password", "", "keystore password")
	secureUnlockCmd.Flags().DurationVar(&secureTTL, "ttl", 10*time.Minute, "secure-mode TTL before automatic expiry")

	secureApproveCmd.Flags().StringVar(&secureTicket, "ticket", "", "ticket ID associated with this privileged action")
	secureApproveCmd.Flags().StringVar(&secureAction, "action", "", "human-readable action description")
	secureApproveCmd.Flags().BoolVar(&secureYes, "yes", false, "skip interactive confirmation prompt")

	secureAnchorSetCmd.Flags().StringVar(&secureTicket, "ticket", "", "ticket ID associated with this privileged action")
	secureAnchorSetCmd.Flags().StringVar(&secureSignerID, "signer-id", "", "trusted signer ID to anchor for this repo")
	secureAnchorSetCmd.Flags().BoolVar(&secureYes, "yes", false, "skip interactive confirmation prompt")

	secureCmd.AddCommand(secureUnlockCmd)
	secureCmd.AddCommand(secureLockCmd)
	secureCmd.AddCommand(secureStatusCmd)
	secureCmd.AddCommand(secureApproveCmd)
	secureCmd.AddCommand(secureAnchorSetCmd)
	rootCmd.AddCommand(secureCmd)
}
