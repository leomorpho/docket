package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/security"
	"github.com/spf13/cobra"
)

func TestSecureModeUnlockExpiryAndPrivilegedRejection(t *testing.T) {
	tmpRepo := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	docketHome = ""
	repo = tmpRepo
	format = "human"

	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(errOut)

	rootCmd.SetArgs([]string{"secure", "approve", "--ticket", "TKT-001", "--action", "set trust anchor", "--yes"})
	err := rootCmd.Execute()
	if !errors.Is(err, security.ErrSecureModeInactive) {
		t.Fatalf("expected secure inactive error before unlock, got: %v", err)
	}

	out.Reset()
	rootCmd.SetArgs([]string{"secure", "unlock", "--password", "pw-1", "--ttl", "120ms"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("secure unlock failed: %v", err)
	}
	if !strings.Contains(out.String(), "Secure mode active until") {
		t.Fatalf("expected unlock output to include expiry, got: %s", out.String())
	}

	out.Reset()
	rootCmd.SetArgs([]string{"secure", "set-anchor", "--ticket", "TKT-001", "--signer-id", "signer-1", "--yes"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("secure set-anchor while active failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpHome, "repos")); err != nil {
		t.Fatalf("expected repo namespace directory to exist after set-anchor: %v", err)
	}

	time.Sleep(220 * time.Millisecond)
	rootCmd.SetArgs([]string{"secure", "set-anchor", "--ticket", "TKT-001", "--signer-id", "signer-1", "--yes"})
	err = rootCmd.Execute()
	if !errors.Is(err, security.ErrSecureModeInactive) {
		t.Fatalf("expected secure inactive error after TTL expiry, got: %v", err)
	}
}

func TestSecureUnlockPromptsAndConfirmsOnFirstUse(t *testing.T) {
	tmpRepo := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	docketHome = tmpHome
	repo = tmpRepo
	format = "human"
	securePassword = ""
	automationMode = false

	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(errOut)

	originalPrompt := securePromptPassword
	defer func() { securePromptPassword = originalPrompt }()

	var prompts []string
	responses := []string{"pw-1", "pw-1"}
	securePromptPassword = func(cmd *cobra.Command, prompt string) (string, error) {
		prompts = append(prompts, prompt)
		resp := responses[0]
		responses = responses[1:]
		return resp, nil
	}

	rootCmd.SetArgs([]string{"secure", "unlock", "--ttl", "5m"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("secure unlock failed: %v", err)
	}

	wantPrompts := []string{"Enter new keystore password: ", "Confirm new keystore password: "}
	if !reflect.DeepEqual(prompts, wantPrompts) {
		t.Fatalf("prompts = %#v, want %#v", prompts, wantPrompts)
	}
	text := errOut.String()
	if !strings.Contains(text, "No secure keystore found in DOCKET_HOME.") {
		t.Fatalf("expected first-use messaging, got: %s", text)
	}
	if !strings.Contains(out.String(), "Secure mode active until") {
		t.Fatalf("expected unlock success output, got: %s", out.String())
	}
}

func TestSecureUnlockExistingKeystorePromptsOnceWithoutSetupMessaging(t *testing.T) {
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	docketHome = tmpHome
	securePassword = ""
	automationMode = false

	ks := security.NewFileKeystore(tmpHome)
	if err := ks.Create("pw-1"); err != nil {
		t.Fatalf("Create keystore failed: %v", err)
	}

	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(errOut)

	originalPrompt := securePromptPassword
	defer func() { securePromptPassword = originalPrompt }()

	var prompts []string
	securePromptPassword = func(cmd *cobra.Command, prompt string) (string, error) {
		prompts = append(prompts, prompt)
		return "pw-1", nil
	}

	rootCmd.SetArgs([]string{"secure", "unlock", "--ttl", "5m"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("secure unlock failed: %v", err)
	}

	wantPrompts := []string{"Enter keystore password: "}
	if !reflect.DeepEqual(prompts, wantPrompts) {
		t.Fatalf("prompts = %#v, want %#v", prompts, wantPrompts)
	}
	if got := errOut.String(); strings.Contains(got, "No secure keystore found in DOCKET_HOME.") {
		t.Fatalf("did not expect first-use setup message for existing keystore, got: %s", got)
	}
}

func TestSecureUnlockUsesExplicitPasswordWithoutPrompting(t *testing.T) {
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	docketHome = tmpHome
	automationMode = false

	ks := security.NewFileKeystore(tmpHome)
	if err := ks.Create("pw-1"); err != nil {
		t.Fatalf("Create keystore failed: %v", err)
	}

	originalPrompt := securePromptPassword
	defer func() { securePromptPassword = originalPrompt }()

	prompted := false
	securePromptPassword = func(cmd *cobra.Command, prompt string) (string, error) {
		prompted = true
		return "", nil
	}

	securePassword = "pw-1"
	defer func() { securePassword = "" }()

	password, err := resolveSecureUnlockPassword(rootCmd)
	if err != nil {
		t.Fatalf("resolveSecureUnlockPassword failed: %v", err)
	}
	if prompted {
		t.Fatal("did not expect interactive prompt when --password is provided")
	}
	if password != "pw-1" {
		t.Fatalf("password = %q, want pw-1", password)
	}
}

func TestSecureUnlockRejectsMismatchedConfirmation(t *testing.T) {
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	docketHome = tmpHome
	securePassword = ""
	automationMode = false

	originalPrompt := securePromptPassword
	defer func() { securePromptPassword = originalPrompt }()

	responses := []string{"pw-1", "pw-2"}
	securePromptPassword = func(cmd *cobra.Command, prompt string) (string, error) {
		resp := responses[0]
		responses = responses[1:]
		return resp, nil
	}

	_, err := resolveSecureUnlockPassword(rootCmd)
	if err == nil || !strings.Contains(err.Error(), "did not match") {
		t.Fatalf("expected confirmation mismatch error, got: %v", err)
	}
}

func TestSecureUnlockRequiresNonInteractiveSecretOutsideTTY(t *testing.T) {
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	docketHome = tmpHome
	securePassword = ""
	automationMode = false

	originalPrompt := securePromptPassword
	defer func() { securePromptPassword = originalPrompt }()
	securePromptPassword = promptSecurePassword

	originalInput := secureInputFile
	defer func() { secureInputFile = originalInput }()
	secureInputFile = func() *os.File {
		f, err := os.CreateTemp(t.TempDir(), "notty")
		if err != nil {
			t.Fatalf("CreateTemp failed: %v", err)
		}
		return f
	}

	_, err := resolveSecureUnlockPassword(rootCmd)
	if err == nil || !strings.Contains(err.Error(), "requires a TTY") {
		t.Fatalf("expected tty error, got: %v", err)
	}
}
