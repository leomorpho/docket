package cmd

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestFixCmd(t *testing.T) {
	tmpDir := t.TempDir()
	oldRepo := repo
	repo = tmpDir
	defer func() { repo = oldRepo }()

	// Initialize git repo
	runGit := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", tmpDir}, args...)...)
		if err := c.Run(); err != nil {
			t.Fatalf("git %v failed: %v", args, err)
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "test")

	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	tkt := &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "Original Title",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: "Original Description",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
	}
	if err := s.CreateTicket(context.Background(), tkt); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	// Commit the ticket
	runGit("add", ".")
	runGit("commit", "-m", "initial ticket")

	// Manually mutate the ticket
	p := filepath.Join(tmpDir, ".docket", "tickets", "TKT-001.md")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	edited := strings.Replace(string(raw), "Original Title", "Mutated Title", 1)
	if err := os.WriteFile(p, []byte(edited), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Verify it's invalid
	errs, _, _ := s.ValidateFile("TKT-001")
	hasSigErr := false
	for _, e := range errs {
		if e.Field == "signature" {
			hasSigErr = true
			break
		}
	}
	if !hasSigErr {
		t.Fatal("expected signature error after mutation")
	}

	// Run fix
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"fix", "TKT-001"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("fix failed: %v", err)
	}

	out := b.String()
	if !strings.Contains(out, "Repairing TKT-001") || !strings.Contains(out, "docket update TKT-001 --title \"Mutated Title\"") {
		t.Fatalf("unexpected fix output: %s", out)
	}

	// Verify it's valid now
	errs, _, _ = s.ValidateFile("TKT-001")
	if len(errs) > 0 {
		t.Fatalf("expected ticket to be valid after fix, got errors: %v", errs)
	}

	// Verify Title is updated
	fixed, _ := s.GetTicket(context.Background(), "TKT-001")
	if fixed.Title != "Mutated Title" {
		t.Fatalf("expected title to be 'Mutated Title', got %q", fixed.Title)
	}
}

func TestFixAll(t *testing.T) {
	tmpDir := t.TempDir()
	oldRepo := repo
	repo = tmpDir
	defer func() { repo = oldRepo }()

	// Initialize git repo
	runGit := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", tmpDir}, args...)...)
		_ = c.Run()
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "test")

	ticket.SaveConfig(tmpDir, ticket.DefaultConfig())
	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-001", Seq: 1, Title: "T1", State: "todo", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me",
		Description: "D", AC: []ticket.AcceptanceCriterion{{Description: "A"}},
	})
	s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-002", Seq: 2, Title: "T2", State: "todo", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me",
		Description: "D", AC: []ticket.AcceptanceCriterion{{Description: "A"}},
	})

	runGit("add", ".")
	runGit("commit", "-m", "initial")

	// Mutate both
	for _, id := range []string{"TKT-001", "TKT-002"} {
		p := filepath.Join(tmpDir, ".docket", "tickets", id+".md")
		raw, _ := os.ReadFile(p)
		edited := strings.Replace(string(raw), "priority: 1", "priority: 2", 1)
		os.WriteFile(p, []byte(edited), 0644)
	}

	// Run fix --all
	// Reset flags
	fixAll = false
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"fix", "--all"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("fix --all failed: %v", err)
	}

	if !strings.Contains(b.String(), "Successfully repaired 2 tickets") {
		t.Fatalf("unexpected fix --all output: %s", b.String())
	}

	// Verify they are valid
	errs1, _, _ := s.ValidateFile("TKT-001")
	errs2, _, _ := s.ValidateFile("TKT-002")
	if len(errs1) > 0 || len(errs2) > 0 {
		t.Fatal("expected tickets to be valid after fix --all")
	}
}
