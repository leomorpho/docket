package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
)

func TestValidateCmd_PrescriptiveForDirectEdit(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "T",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	p := filepath.Join(tmpDir, ".docket", "tickets", "TKT-001.md")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	edited := strings.Replace(string(raw), "state: todo", "state: done", 1)
	if err := os.WriteFile(p, []byte(edited), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"validate", "TKT-001"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected validate to fail on direct edit")
	}
	out := b.String()
	if !strings.Contains(out, "Use: docket update TKT-001 --state done") {
		t.Fatalf("expected prescriptive command in validate output, got:\n%s", out)
	}
}
