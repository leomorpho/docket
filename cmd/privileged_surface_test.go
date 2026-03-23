package cmd

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/security"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestLockReleaseRequiresSecureSurface(t *testing.T) {
	tmpRepo := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	docketHome = ""
	repo = tmpRepo
	format = "human"
	cfg := ticket.DefaultConfig()
	cfg.SecurityEnforcement = true
	if err := ticket.SaveConfig(tmpRepo, cfg); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	if err := upsertLock(tmpRepo, fileLock{
		TicketID:     "TKT-500",
		WorktreePath: tmpRepo,
		Files:        []string{"x.go"},
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("upsert lock failed: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"lock", "release", "TKT-500", "--ticket", "TKT-185", "--yes"})
	err := rootCmd.Execute()
	if !errors.Is(err, security.ErrSecureModeInactive) {
		t.Fatalf("expected secure-mode inactive rejection, got: %v", err)
	}

	rootCmd.SetArgs([]string{"secure", "unlock", "--password", "pw-1", "--ttl", "5m"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("secure unlock failed: %v", err)
	}
	rootCmd.SetArgs([]string{"lock", "release", "TKT-500", "--ticket", "TKT-185", "--yes"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("lock release after secure unlock failed: %v", err)
	}
}

func TestLockReleaseAutoUnlocksWithEnvPassword(t *testing.T) {
	tmpRepo := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	t.Setenv("DOCKET_KEYSTORE_PASSWORD", "pw-1")
	docketHome = ""
	repo = tmpRepo
	format = "human"
	cfg := ticket.DefaultConfig()
	cfg.SecurityEnforcement = true
	if err := ticket.SaveConfig(tmpRepo, cfg); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	ks := security.NewFileKeystore(tmpHome)
	if err := ks.Create("pw-1"); err != nil {
		t.Fatalf("create keystore failed: %v", err)
	}
	if err := upsertLock(tmpRepo, fileLock{
		TicketID:     "TKT-501",
		WorktreePath: tmpRepo,
		Files:        []string{"x.go"},
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("upsert lock failed: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"lock", "release", "TKT-501", "--ticket", "TKT-185", "--yes"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("lock release with env auto-unlock failed: %v", err)
	}

	session := security.NewSessionManager(tmpHome)
	if err := session.RequireActive(tmpRepo); err != nil {
		t.Fatalf("expected secure session to be active after auto-unlock, got: %v", err)
	}
}
