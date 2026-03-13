package git

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHasTicketTrailerSince(t *testing.T) {
	repo := setupGitRepo(t)

	if err := os.WriteFile(filepath.Join(repo, "f.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGitCmd(t, repo, "add", ".")
	runGitCmd(t, repo, "commit", "-m", "chore: no ticket")

	since := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	ok, err := HasTicketTrailerSince(repo, "HEAD", "TKT-198", since)
	if err != nil {
		t.Fatalf("HasTicketTrailerSince failed: %v", err)
	}
	if ok {
		t.Fatalf("expected no ticket-linked commit yet")
	}

	if err := os.WriteFile(filepath.Join(repo, "f.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGitCmd(t, repo, "add", ".")
	runGitCmd(t, repo, "commit", "-m", "feat: link ticket\n\nTicket: TKT-198")

	ok, err = HasTicketTrailerSince(repo, "HEAD", "TKT-198", since)
	if err != nil {
		t.Fatalf("HasTicketTrailerSince failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected ticket-linked commit to be detected")
	}
}
