package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func writeProofPNGFixture(t *testing.T, path string) {
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
		t.Fatalf("write png fixture: %v", err)
	}
}

func setupProofTicket(t *testing.T, repoRoot string) {
	t.Helper()
	ticket.SaveConfig(repoRoot, ticket.DefaultConfig())
	now := time.Now().UTC().Truncate(time.Second)
	s := local.New(repoRoot)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "Proof command test",
		State:       ticket.State("in-progress"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "sufficient context for proof command integration tests",
		AC:          []ticket.AcceptanceCriterion{{Description: "proof command works"}},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
}

func TestProofCmd_AddListRemoveJSONFlow(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	setupProofTicket(t, tmpDir)

	fixtureRel := filepath.Join("fixtures", "proof.png")
	fixtureAbs := filepath.Join(tmpDir, fixtureRel)
	writeProofPNGFixture(t, fixtureAbs)

	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(errOut)

	rootCmd.SetArgs([]string{"proof", "add", "TKT-001", "--file", fixtureRel, "--proof-title", "Before", "--note", "Baseline screenshot", "--captured-at", "2026-03-16T18:40:00Z"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("proof add failed: %v; stderr=%s", err, errOut.String())
	}
	var addPayload struct {
		TicketID string `json:"ticket_id"`
		Proof    struct {
			ID         string `json:"id"`
			ProofTitle string `json:"proof_title"`
			Note       string `json:"note"`
			AddedAt    string `json:"added_at"`
		} `json:"proof"`
	}
	if err := json.Unmarshal(out.Bytes(), &addPayload); err != nil {
		t.Fatalf("parse add json: %v; body=%s", err, out.String())
	}
	if addPayload.TicketID != "TKT-001" || addPayload.Proof.ID == "" {
		t.Fatalf("unexpected add payload: %+v", addPayload)
	}
	if addPayload.Proof.ProofTitle != "Before" || addPayload.Proof.Note != "Baseline screenshot" || addPayload.Proof.AddedAt == "" {
		t.Fatalf("expected proof_title/note/added_at in add payload, got %+v", addPayload.Proof)
	}

	out.Reset()
	errOut.Reset()
	rootCmd.SetArgs([]string{"proof", "list", "TKT-001"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("proof list failed: %v; stderr=%s", err, errOut.String())
	}
	var listPayload struct {
		Proofs []struct {
			ID string `json:"id"`
		} `json:"proofs"`
	}
	if err := json.Unmarshal(out.Bytes(), &listPayload); err != nil {
		t.Fatalf("parse list json: %v; body=%s", err, out.String())
	}
	if len(listPayload.Proofs) != 1 || listPayload.Proofs[0].ID != addPayload.Proof.ID {
		t.Fatalf("unexpected list payload: %+v", listPayload)
	}

	out.Reset()
	errOut.Reset()
	rootCmd.SetArgs([]string{"proof", "remove", "TKT-001", "--proof-id", addPayload.Proof.ID})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("proof remove failed: %v; stderr=%s", err, errOut.String())
	}
	var removePayload struct {
		Removed struct {
			ID string `json:"id"`
		} `json:"removed"`
	}
	if err := json.Unmarshal(out.Bytes(), &removePayload); err != nil {
		t.Fatalf("parse remove json: %v; body=%s", err, out.String())
	}
	if removePayload.Removed.ID != addPayload.Proof.ID {
		t.Fatalf("unexpected removed id: %+v", removePayload)
	}

	out.Reset()
	errOut.Reset()
	rootCmd.SetArgs([]string{"proof", "list", "TKT-001"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("proof list after remove failed: %v; stderr=%s", err, errOut.String())
	}
	listPayload = struct {
		Proofs []struct {
			ID string `json:"id"`
		} `json:"proofs"`
	}{}
	if err := json.Unmarshal(out.Bytes(), &listPayload); err != nil {
		t.Fatalf("parse list-after-remove json: %v; body=%s", err, out.String())
	}
	if len(listPayload.Proofs) != 0 {
		t.Fatalf("expected no proofs after remove, got %+v", listPayload)
	}
}

func TestProofCmd_AddValidationErrorEnvelopeJSON(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	setupProofTicket(t, tmpDir)

	fixtureRel := filepath.Join("fixtures", "proof.png")
	fixtureAbs := filepath.Join(tmpDir, fixtureRel)
	writeProofPNGFixture(t, fixtureAbs)

	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(errOut)

	rootCmd.SetArgs([]string{"proof", "add", "TKT-001", "--file", fixtureRel})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected proof add validation error")
	}
	var payload map[string]any
	if unmarshalErr := json.Unmarshal(out.Bytes(), &payload); unmarshalErr != nil {
		t.Fatalf("parse error payload: %v; body=%s", unmarshalErr, out.String())
	}
	envAny, ok := payload["error_envelope"].(map[string]any)
	if !ok {
		t.Fatalf("missing error_envelope payload: %+v", payload)
	}
	if envAny["error_code"] == "" || envAny["field"] == "" || envAny["suggested_fix"] == "" {
		t.Fatalf("expected machine-readable envelope fields, got %+v", envAny)
	}
}

func TestProofCmd_RemoveNotFoundEnvelopeJSON(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	setupProofTicket(t, tmpDir)

	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(errOut)

	rootCmd.SetArgs([]string{"proof", "remove", "TKT-001", "--proof-id", "PRF-does-not-exist"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected proof remove not-found error")
	}
	var payload map[string]any
	if unmarshalErr := json.Unmarshal(out.Bytes(), &payload); unmarshalErr != nil {
		t.Fatalf("parse error payload: %v; body=%s", unmarshalErr, out.String())
	}
	envAny, ok := payload["error_envelope"].(map[string]any)
	if !ok {
		t.Fatalf("missing error_envelope payload: %+v", payload)
	}
	if envAny["error_code"] != "not_found" {
		t.Fatalf("expected not_found error code, got %+v", envAny)
	}
	if envAny["field"] != "proof_id" {
		t.Fatalf("expected proof_id field, got %+v", envAny)
	}
}
