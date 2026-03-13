package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/leomorpho/docket/internal/security"
	"github.com/leomorpho/docket/internal/workflow"
	"github.com/spf13/cobra"
)

var (
	workflowProposalPath string
	workflowSignerID     string
	workflowTicketID     string
	workflowConfirmYes   bool
)

var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Manage runtime workflow proposal and lock artifacts",
}

var workflowLockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Generate and validate signed workflow.lock artifacts",
}

var workflowLockGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate signed workflow.lock from editable proposal",
	RunE: func(cmd *cobra.Command, args []string) error {
		if workflowSignerID == "" {
			return fmt.Errorf("--signer-id is required")
		}
		if workflowTicketID == "" {
			return fmt.Errorf("--ticket is required")
		}

		session := security.NewSessionManager(docketHome)
		if err := session.RequireActive(repo); err != nil {
			return err
		}

		action := "generate workflow.lock"
		if !workflowConfirmYes {
			ok, err := security.ConfirmPrivilegedAction(cmd.InOrStdin(), cmd.OutOrStdout(), repo, workflowTicketID, action)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("privileged action cancelled")
			}
		}

		ksProvider, err := keystoreProvider()
		if err != nil {
			return err
		}
		ks, ok := ksProvider.(*security.FileKeystore)
		if !ok {
			return fmt.Errorf("unsupported keystore provider")
		}
		pw := os.Getenv("DOCKET_KEYSTORE_PASSWORD")
		if pw == "" {
			return fmt.Errorf("DOCKET_KEYSTORE_PASSWORD is required for signing workflow.lock")
		}
		err = ks.Unlock(pw)
		if errors.Is(err, security.ErrKeystoreNotFound) {
			if err := ks.Create(pw); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		lock, err := workflow.GenerateWorkflowLock(repo, workflowProposalPath, workflowSignerID, ks)
		if err != nil {
			return err
		}
		if err := workflow.WriteWorkflowLock(repo, lock); err != nil {
			return err
		}
		if err := session.RecordPrivilegedAction(repo, workflowTicketID, action); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Generated %s (proposal hash %s)\n", workflow.DefaultWorkflowLock, lock.ProposalHash)
		return nil
	},
}

var workflowLockValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate workflow.lock integrity against proposal and signature metadata",
	RunE: func(cmd *cobra.Command, args []string) error {
		lock, err := workflow.ParseWorkflowLock(repo)
		if err != nil {
			return err
		}
		if err := workflow.ValidateWorkflowLock(repo, lock); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s is valid and current.\n", workflow.DefaultWorkflowLock)
		return nil
	},
}

func init() {
	workflowLockGenerateCmd.Flags().StringVar(&workflowProposalPath, "proposal", workflow.DefaultWorkflowPolicy, "path to editable workflow proposal file")
	workflowLockGenerateCmd.Flags().StringVar(&workflowSignerID, "signer-id", "", "signer identifier for lock metadata")
	workflowLockGenerateCmd.Flags().StringVar(&workflowTicketID, "ticket", "", "ticket ID authorizing this privileged lock generation")
	workflowLockGenerateCmd.Flags().BoolVar(&workflowConfirmYes, "yes", false, "skip interactive confirmation prompt")

	workflowLockValidateCmd.Flags().StringVar(&workflowProposalPath, "proposal", workflow.DefaultWorkflowPolicy, "reserved for compatibility; lock stores proposal path")

	workflowLockCmd.AddCommand(workflowLockGenerateCmd)
	workflowLockCmd.AddCommand(workflowLockValidateCmd)
	workflowCmd.AddCommand(workflowLockCmd)
	rootCmd.AddCommand(workflowCmd)
}
