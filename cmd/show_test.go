package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/proof"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func writeShowProofPNG(t *testing.T, path string) {
	t.Helper()
	data := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde,
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture dir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func TestShowCmd(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	// 0. Setup
	s := local.New(tmpDir)
	ticket.SaveConfig(tmpDir, ticket.DefaultConfig())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	tick := &ticket.Ticket{
		ID:          "TKT-001",
		Title:       "Test Ticket",
		State:       ticket.State("todo"),
		Priority:    1,
		Description: "Desc here",
		AC:          []ticket.AcceptanceCriterion{{Description: "AC1", Done: true}},
		Plan:        []ticket.PlanStep{{Description: "Step 1", Status: "pending"}},
		Comments:    []ticket.Comment{{Author: "me", Body: "hello"}},
		Handoff:     "some handoff",
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
	}
	s.CreateTicket(ctx, tick)
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-002",
		Title:       "Child Ticket",
		Parent:      "TKT-001",
		State:       ticket.State("todo"),
		Priority:    2,
		Description: "Child desc",
		AC: []ticket.AcceptanceCriterion{
			{Description: "AC child 1", Done: true},
			{Description: "AC child 2", Done: false},
		},
		CreatedAt: now.Add(time.Minute),
		UpdatedAt: now.Add(time.Minute),
		CreatedBy: "me",
	}); err != nil {
		t.Fatalf("create child ticket failed: %v", err)
	}

	// 1. Human format
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"show", "TKT-001"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show failed: %v", err)
	}
	if !strings.Contains(b.String(), "TKT-001 · todo") {
		t.Errorf("expected header, got:\n%s", b.String())
	}
	if !strings.Contains(b.String(), "Test Ticket") {
		t.Errorf("expected title, got:\n%s", b.String())
	}
	if !strings.Contains(b.String(), "Descendant AC: 1/2 done across 1 descendant tickets") {
		t.Errorf("expected descendant AC aggregation in show output, got:\n%s", b.String())
	}

	// 2. MD format
	b.Reset()
	format = "md"
	rootCmd.SetArgs([]string{"show", "TKT-001", "--format", "md"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show md failed: %v", err)
	}
	if !strings.HasPrefix(b.String(), "---") {
		t.Errorf("expected raw MD starting with ---, got:\n%s", b.String())
	}

	// 3. JSON format
	b.Reset()
	format = "json"
	rootCmd.SetArgs([]string{"show", "TKT-001", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show json failed: %v", err)
	}
	var res map[string]interface{}
	if err := json.Unmarshal(b.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if res["id"] != "TKT-001" {
		t.Errorf("expected ID TKT-001, got: %v", res["id"])
	}

	// 4. Context format
	b.Reset()
	format = "context"
	rootCmd.SetArgs([]string{"show", "TKT-001", "--format", "context"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show context failed: %v", err)
	}
	if !strings.Contains(b.String(), "TICKET: TKT-001") {
		t.Errorf("expected compact context output, got:\n%s", b.String())
	}

	// 5. Not found
	b.Reset()
	format = "human"
	rootCmd.SetArgs([]string{"show", "TKT-999"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent ticket, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestShowCmd_ProofMetadataAppearsWhenPresent(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	ticket.SaveConfig(tmpDir, ticket.DefaultConfig())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	tick := &ticket.Ticket{
		ID:          "TKT-001",
		Title:       "Proof Ticket",
		State:       ticket.State("todo"),
		Priority:    1,
		Description: "Desc here",
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		AC:          []ticket.AcceptanceCriterion{{Description: "AC1", Done: true}},
	}
	if err := s.CreateTicket(ctx, tick); err != nil {
		t.Fatalf("create ticket failed: %v", err)
	}

	proofRel := filepath.Join("fixtures", "proof.png")
	proofAbs := filepath.Join(tmpDir, proofRel)
	writeShowProofPNG(t, proofAbs)
	if _, err := s.AddProof(ctx, proof.AddInput{
		TicketID:   "TKT-001",
		SourcePath: proofRel,
		ProofTitle: "Before",
		Note:       "baseline proof",
		AddedAt:    now.Add(time.Minute).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("add proof failed: %v", err)
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"show", "TKT-001"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show human failed: %v", err)
	}
	if !strings.Contains(b.String(), "Proofs (1)") || !strings.Contains(b.String(), "baseline proof") {
		t.Fatalf("expected proof summary in human output, got:\n%s", b.String())
	}

	b.Reset()
	format = "json"
	rootCmd.SetArgs([]string{"show", "TKT-001", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show json failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(b.Bytes(), &payload); err != nil {
		t.Fatalf("parse show json failed: %v", err)
	}
	proofsAny, ok := payload["proofs"].([]any)
	if !ok || len(proofsAny) != 1 {
		t.Fatalf("expected one proof entry in json output, got %+v", payload["proofs"])
	}

	b.Reset()
	format = "context"
	rootCmd.SetArgs([]string{"show", "TKT-001", "--format", "context"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show context failed: %v", err)
	}
	if !strings.Contains(b.String(), "PROOFS:") {
		t.Fatalf("expected PROOFS summary in context output, got:\n%s", b.String())
	}
}
