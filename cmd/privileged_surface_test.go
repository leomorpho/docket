package cmd

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/security"
)

func TestLockReleaseRequiresSecureSurface(t *testing.T) {
	tmpRepo := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	docketHome = ""
	repo = tmpRepo
	format = "human"

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
