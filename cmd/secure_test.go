package cmd

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/security"
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
	rootCmd.SetArgs([]string{"secure", "approve", "--ticket", "TKT-001", "--action", "set trust anchor", "--yes"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("secure approve while active failed: %v", err)
	}

	time.Sleep(220 * time.Millisecond)
	rootCmd.SetArgs([]string{"secure", "approve", "--ticket", "TKT-001", "--action", "set trust anchor", "--yes"})
	err = rootCmd.Execute()
	if !errors.Is(err, security.ErrSecureModeInactive) {
		t.Fatalf("expected secure inactive error after TTL expiry, got: %v", err)
	}
}
