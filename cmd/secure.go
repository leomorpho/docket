package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	term "github.com/charmbracelet/x/term"
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

var (
	securePromptPassword = promptSecurePassword
	secureInputFile      = func() *os.File { return os.Stdin }
)

var secureCmd = &cobra.Command{
	Use:   "secure",
	Short: "Manage secure-mode session for privileged operations",
}

var secureUnlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Unlock secure mode with a password and TTL",
	RunE: func(cmd *cobra.Command, args []string) error {
		password, err := resolveSecureUnlockPassword(cmd)
		if err != nil {
			return err
		}
		mgr := security.NewSessionManager(docketHome)
		if err := mgr.Unlock(repo, password, secureTTL); err != nil {
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
		if err := ensureSecureSessionActive(repo); err != nil {
			return err
		}

		if err := confirmPrivilegedPrompt(cmd, secureYes, secureTicket, secureAction); err != nil {
			return err
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
		if err := ensureSecureSessionActive(repo); err != nil {
			return err
		}

		action := fmt.Sprintf("set trust anchor signer=%s", secureSignerID)
		if err := confirmPrivilegedPrompt(cmd, secureYes, secureTicket, action); err != nil {
			return err
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
	secureUnlockCmd.Flags().StringVar(&securePassword, "password", "", "keystore password (non-interactive; less secure than prompt entry)")
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

func resolveSecureUnlockPassword(cmd *cobra.Command) (string, error) {
	if securePassword != "" {
		return securePassword, nil
	}
	if envPassword := strings.TrimSpace(os.Getenv("DOCKET_KEYSTORE_PASSWORD")); envPassword != "" {
		return envPassword, nil
	}
	if isAutomationMode() {
		return "", fmt.Errorf("automation mode requires --password or DOCKET_KEYSTORE_PASSWORD for secure unlock")
	}

	ks := security.NewFileKeystore(docketHome)
	firstUse := false
	if _, err := os.Stat(ks.Path()); err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("checking secure keystore: %w", err)
		}
		firstUse = true
	}

	if firstUse {
		securityMode, securityNote := securityEnforcementSurface(repo)
		fmt.Fprintln(cmd.ErrOrStderr(), "No secure keystore found in DOCKET_HOME.")
		fmt.Fprintln(cmd.ErrOrStderr(), "This will create a local keystore for secure-mode operations.")
		fmt.Fprintln(cmd.ErrOrStderr(), "Choose a password you will need again to unlock secure mode on this machine.")
		fmt.Fprintf(cmd.ErrOrStderr(), "Security enforcement: %s.\n", securityMode)
		fmt.Fprintf(cmd.ErrOrStderr(), "Enforcement note: %s\n", securityNote)
	}

	password, err := securePromptPassword(cmd, promptLabel(firstUse, false))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(password) == "" {
		return "", fmt.Errorf("keystore password cannot be empty")
	}
	if !firstUse {
		return password, nil
	}

	confirm, err := securePromptPassword(cmd, promptLabel(true, true))
	if err != nil {
		return "", err
	}
	if password != confirm {
		return "", fmt.Errorf("keystore password confirmation did not match")
	}
	return password, nil
}

func promptLabel(firstUse, confirm bool) string {
	if firstUse {
		if confirm {
			return "Confirm new keystore password: "
		}
		return "Enter new keystore password: "
	}
	return "Enter keystore password: "
}

func promptSecurePassword(cmd *cobra.Command, prompt string) (string, error) {
	in := secureInputFile()
	if in == nil {
		return "", fmt.Errorf("interactive secure unlock requires a terminal")
	}
	if !term.IsTerminal(in.Fd()) {
		return "", fmt.Errorf("interactive secure unlock requires a TTY; use --password or DOCKET_KEYSTORE_PASSWORD for non-interactive use")
	}
	return readMaskedPassword(in, cmd.ErrOrStderr(), prompt)
}

func readMaskedPassword(in *os.File, out io.Writer, prompt string) (string, error) {
	if _, err := fmt.Fprint(out, prompt); err != nil {
		return "", err
	}

	oldState, err := term.GetState(in.Fd())
	if err != nil {
		return "", err
	}
	if _, err := term.MakeRaw(in.Fd()); err != nil {
		return "", err
	}
	defer func() {
		_ = term.Restore(in.Fd(), oldState)
		_, _ = fmt.Fprintln(out)
	}()

	buf := make([]byte, 0, 64)
	readBuf := make([]byte, 1)
	for {
		if _, err := in.Read(readBuf); err != nil {
			return "", err
		}
		b := readBuf[0]
		switch b {
		case '\r', '\n':
			return string(buf), nil
		case 3:
			return "", errors.New("secure unlock cancelled")
		case 127, 8:
			if len(buf) == 0 {
				continue
			}
			buf = buf[:len(buf)-1]
			if _, err := fmt.Fprint(out, "\b \b"); err != nil {
				return "", err
			}
		default:
			if b < 32 {
				continue
			}
			buf = append(buf, b)
			if _, err := fmt.Fprint(out, "*"); err != nil {
				return "", err
			}
		}
	}
}
