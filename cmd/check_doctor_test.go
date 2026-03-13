package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestCheckDoctor(t *testing.T) {
	tmpDir := t.TempDir()
	oldRepo := repo
	repo = tmpDir
	defer func() { repo = oldRepo }()

	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	
	// Create a valid ticket
	s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-001", Seq: 1, Title: "T1", State: "todo", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me",
		Description: "This is a long enough description for the doctor to not complain about it.",
		AC: []ticket.AcceptanceCriterion{{Description: "A"}, {Description: "B"}},
	})

	// Create an invalid ticket (manual edit)
	s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-002", Seq: 2, Title: "T2", State: "todo", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me",
		Description: "D", AC: []ticket.AcceptanceCriterion{{Description: "A"}},
	})
	p := filepath.Join(tmpDir, ".docket", "tickets", "TKT-002.md")
	raw, _ := os.ReadFile(p)
	edited := strings.Replace(string(raw), "T2", "Mutated", 1)
	os.WriteFile(p, []byte(edited), 0644)

	// Run check --doctor
	checkDoctor = false
	checkFix = false
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"check", "--doctor"})
	
	// Execute might return error if findings exist, handle it
	_ = rootCmd.Execute()

	out := b.String()
	if !strings.Contains(out, "🚨 Direct Mutation Detected. Run `docket fix TKT-002` to repair.") {
		t.Fatalf("expected doctor to find mutation, got:\n%s", out)
	}
}
