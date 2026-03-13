package cmd

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/leomorpho/docket/internal/security"
	"github.com/spf13/cobra"
)

var (
	identityPath     string
	identityDeviceID string
	identityPublic   string
	identityTicket   string
	identityYes      bool
)

var secureIdentityCmd = &cobra.Command{
	Use:   "identity",
	Short: "Manage local identity metadata (export, enrollment, rotation, recovery)",
}

var secureIdentityExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export public identity metadata for migration or backup",
	RunE: func(cmd *cobra.Command, args []string) error {
		if identityPath == "" {
			return fmt.Errorf("--path is required")
		}
		im, err := ensureIdentityManager()
		if err != nil {
			return err
		}
		if err := im.ExportTo(identityPath); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Identity metadata exported to %s\n", identityPath)
		return nil
	},
}

var secureIdentityRecoverCmd = &cobra.Command{
	Use:   "recover",
	Short: "Recover identity metadata from an exported file",
	RunE: func(cmd *cobra.Command, args []string) error {
		if identityPath == "" {
			return fmt.Errorf("--path is required")
		}
		if identityTicket == "" {
			return fmt.Errorf("--ticket is required")
		}
		mgr := security.NewSessionManager(docketHome)
		if err := mgr.RequireActive(repo); err != nil {
			return err
		}
		if !identityYes {
			ok, err := security.ConfirmPrivilegedAction(cmd.InOrStdin(), cmd.OutOrStdout(), repo, identityTicket, "recover identity metadata")
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("privileged action cancelled")
			}
		}
		im, err := ensureIdentityManager()
		if err != nil {
			return err
		}
		if err := im.RecoverFrom(identityPath); err != nil {
			return err
		}
		if err := mgr.RecordPrivilegedAction(repo, identityTicket, "recover identity metadata"); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Identity metadata recovered.")
		return nil
	},
}

var secureIdentityEnrollCmd = &cobra.Command{
	Use:   "enroll",
	Short: "Enroll another device public key",
	RunE: func(cmd *cobra.Command, args []string) error {
		if identityTicket == "" {
			return fmt.Errorf("--ticket is required")
		}
		if identityDeviceID == "" {
			return fmt.Errorf("--device-id is required")
		}
		pub, err := parsePublicKey(identityPublic)
		if err != nil {
			return err
		}
		mgr := security.NewSessionManager(docketHome)
		if err := mgr.RequireActive(repo); err != nil {
			return err
		}
		action := fmt.Sprintf("enroll device key %s", identityDeviceID)
		if !identityYes {
			ok, err := security.ConfirmPrivilegedAction(cmd.InOrStdin(), cmd.OutOrStdout(), repo, identityTicket, action)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("privileged action cancelled")
			}
		}
		im, err := ensureIdentityManager()
		if err != nil {
			return err
		}
		if err := im.EnrollDevice(identityDeviceID, pub); err != nil {
			return err
		}
		if err := mgr.RecordPrivilegedAction(repo, identityTicket, action); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Enrolled device %s\n", identityDeviceID)
		return nil
	},
}

var secureIdentityRevokeCmd = &cobra.Command{
	Use:   "revoke",
	Short: "Revoke an enrolled device key",
	RunE: func(cmd *cobra.Command, args []string) error {
		if identityTicket == "" {
			return fmt.Errorf("--ticket is required")
		}
		if identityDeviceID == "" {
			return fmt.Errorf("--device-id is required")
		}
		mgr := security.NewSessionManager(docketHome)
		if err := mgr.RequireActive(repo); err != nil {
			return err
		}
		action := fmt.Sprintf("revoke device key %s", identityDeviceID)
		if !identityYes {
			ok, err := security.ConfirmPrivilegedAction(cmd.InOrStdin(), cmd.OutOrStdout(), repo, identityTicket, action)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("privileged action cancelled")
			}
		}
		im, err := ensureIdentityManager()
		if err != nil {
			return err
		}
		if err := im.RevokeDevice(identityDeviceID); err != nil {
			return err
		}
		if err := mgr.RecordPrivilegedAction(repo, identityTicket, action); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Revoked device %s\n", identityDeviceID)
		return nil
	},
}

var secureIdentityRotateCmd = &cobra.Command{
	Use:   "rotate",
	Short: "Rotate current device to a newly enrolled key",
	RunE: func(cmd *cobra.Command, args []string) error {
		if identityTicket == "" {
			return fmt.Errorf("--ticket is required")
		}
		if identityDeviceID == "" {
			return fmt.Errorf("--device-id is required")
		}
		pub, err := parsePublicKey(identityPublic)
		if err != nil {
			return err
		}
		mgr := security.NewSessionManager(docketHome)
		if err := mgr.RequireActive(repo); err != nil {
			return err
		}
		action := fmt.Sprintf("rotate current device key to %s", identityDeviceID)
		if !identityYes {
			ok, err := security.ConfirmPrivilegedAction(cmd.InOrStdin(), cmd.OutOrStdout(), repo, identityTicket, action)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("privileged action cancelled")
			}
		}
		im, err := ensureIdentityManager()
		if err != nil {
			return err
		}
		if err := im.RotateCurrentDevice(identityDeviceID, pub); err != nil {
			return err
		}
		if err := mgr.RecordPrivilegedAction(repo, identityTicket, action); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Rotated current device to %s\n", identityDeviceID)
		return nil
	},
}

func parsePublicKey(encoded string) (ed25519.PublicKey, error) {
	if encoded == "" {
		return nil, fmt.Errorf("--public-key is required")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decoding --public-key: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("--public-key must decode to %d bytes", ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}

func ensureIdentityManager() (*security.IdentityManager, error) {
	ksProvider, err := keystoreProvider()
	if err != nil {
		return nil, err
	}
	ks, ok := ksProvider.(*security.FileKeystore)
	if !ok {
		return nil, fmt.Errorf("unsupported keystore provider")
	}
	if err := ks.Unlock(securePasswordFromEnv()); err != nil {
		return nil, err
	}
	im := security.NewIdentityManager(docketHome)
	if _, err := im.EnsureInitialized(ks); err != nil {
		return nil, err
	}
	return im, nil
}

// securePasswordFromEnv allows non-interactive identity export/recovery tooling.
func securePasswordFromEnv() string {
	return os.Getenv("DOCKET_KEYSTORE_PASSWORD")
}

func init() {
	secureIdentityExportCmd.Flags().StringVar(&identityPath, "path", "", "destination path for exported metadata")

	secureIdentityRecoverCmd.Flags().StringVar(&identityPath, "path", "", "source path containing exported metadata")
	secureIdentityRecoverCmd.Flags().StringVar(&identityTicket, "ticket", "", "ticket ID for privileged recovery action")
	secureIdentityRecoverCmd.Flags().BoolVar(&identityYes, "yes", false, "skip interactive confirmation prompt")

	secureIdentityEnrollCmd.Flags().StringVar(&identityTicket, "ticket", "", "ticket ID for privileged enrollment action")
	secureIdentityEnrollCmd.Flags().StringVar(&identityDeviceID, "device-id", "", "new device identifier")
	secureIdentityEnrollCmd.Flags().StringVar(&identityPublic, "public-key", "", "base64-encoded Ed25519 public key")
	secureIdentityEnrollCmd.Flags().BoolVar(&identityYes, "yes", false, "skip interactive confirmation prompt")

	secureIdentityRevokeCmd.Flags().StringVar(&identityTicket, "ticket", "", "ticket ID for privileged revocation action")
	secureIdentityRevokeCmd.Flags().StringVar(&identityDeviceID, "device-id", "", "device identifier to revoke")
	secureIdentityRevokeCmd.Flags().BoolVar(&identityYes, "yes", false, "skip interactive confirmation prompt")

	secureIdentityRotateCmd.Flags().StringVar(&identityTicket, "ticket", "", "ticket ID for privileged rotation action")
	secureIdentityRotateCmd.Flags().StringVar(&identityDeviceID, "device-id", "", "new active device identifier")
	secureIdentityRotateCmd.Flags().StringVar(&identityPublic, "public-key", "", "base64-encoded Ed25519 public key")
	secureIdentityRotateCmd.Flags().BoolVar(&identityYes, "yes", false, "skip interactive confirmation prompt")

	secureIdentityCmd.AddCommand(secureIdentityExportCmd)
	secureIdentityCmd.AddCommand(secureIdentityRecoverCmd)
	secureIdentityCmd.AddCommand(secureIdentityEnrollCmd)
	secureIdentityCmd.AddCommand(secureIdentityRevokeCmd)
	secureIdentityCmd.AddCommand(secureIdentityRotateCmd)
	secureCmd.AddCommand(secureIdentityCmd)
}
