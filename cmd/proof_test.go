package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/lifecycle"
	"github.com/leomorpho/docket/internal/proof"
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

func TestProofListFiltersAndOrderingJSON(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	setupProofTicket(t, tmpDir)

	fixtureRel := filepath.Join("fixtures", "proof.png")
	fixtureAbs := filepath.Join(tmpDir, fixtureRel)
	writeProofPNGFixture(t, fixtureAbs)

	s := local.New(tmpDir)
	ctx := context.Background()
	seed := []proof.AddInput{
		{TicketID: "TKT-001", SourcePath: fixtureRel, ProofTitle: "old-a", Note: "a", AddedAt: "2026-03-16T18:00:00Z", Actor: "agent:a"},
		{TicketID: "TKT-001", SourcePath: fixtureRel, ProofTitle: "new-b", Note: "b", AddedAt: "2026-03-16T18:10:00Z", Actor: "agent:b"},
		{TicketID: "TKT-001", SourcePath: fixtureRel, ProofTitle: "mid-a", Note: "c", AddedAt: "2026-03-16T18:05:00Z", Actor: "agent:a"},
	}
	for _, in := range seed {
		if _, err := s.AddProof(ctx, in); err != nil {
			t.Fatalf("seed add proof failed: %v", err)
		}
	}

	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(errOut)

	rootCmd.SetArgs([]string{"proof", "list", "TKT-001"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("proof list default failed: %v", err)
	}
	var base struct {
		Proofs []struct {
			ProofTitle string `json:"proof_title"`
			AddedAt    string `json:"added_at"`
			Actor      string `json:"actor"`
		} `json:"proofs"`
	}
	if err := json.Unmarshal(out.Bytes(), &base); err != nil {
		t.Fatalf("parse base list json: %v; body=%s", err, out.String())
	}
	if len(base.Proofs) != 3 {
		t.Fatalf("expected 3 proofs in base list, got %d", len(base.Proofs))
	}
	if base.Proofs[0].ProofTitle != "new-b" || base.Proofs[1].ProofTitle != "mid-a" || base.Proofs[2].ProofTitle != "old-a" {
		t.Fatalf("expected newest-first deterministic ordering, got %+v", base.Proofs)
	}

	out.Reset()
	rootCmd.SetArgs([]string{"proof", "list", "TKT-001", "--since", "2026-03-16T18:05:00Z", "--actor", "agent:a", "--limit", "1", "--kind", "screenshot"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("proof list filtered failed: %v", err)
	}
	var filtered struct {
		Proofs []struct {
			ProofTitle string `json:"proof_title"`
			Actor      string `json:"actor"`
		} `json:"proofs"`
	}
	if err := json.Unmarshal(out.Bytes(), &filtered); err != nil {
		t.Fatalf("parse filtered list json: %v; body=%s", err, out.String())
	}
	if len(filtered.Proofs) != 1 || filtered.Proofs[0].ProofTitle != "mid-a" || filtered.Proofs[0].Actor != "agent:a" {
		t.Fatalf("unexpected filtered proofs payload: %+v", filtered)
	}
}

func TestProofListFilterValidationEnvelopeJSON(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	setupProofTicket(t, tmpDir)

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(new(bytes.Buffer))

	rootCmd.SetArgs([]string{"proof", "list", "TKT-001", "--kind", "binary"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected invalid kind validation error")
	}
	var payload map[string]any
	if unmarshalErr := json.Unmarshal(out.Bytes(), &payload); unmarshalErr != nil {
		t.Fatalf("parse error payload: %v; body=%s", unmarshalErr, out.String())
	}
	envAny, ok := payload["error_envelope"].(map[string]any)
	if !ok {
		t.Fatalf("missing error_envelope payload: %+v", payload)
	}
	if envAny["field"] != "kind" {
		t.Fatalf("expected kind field validation error, got %+v", envAny)
	}
}

func TestProofRemoveAndGCSharedBlobSafetyJSON(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	setupProofTicket(t, tmpDir)

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-002",
		Seq:         2,
		Title:       "Proof command test two",
		State:       ticket.State("in-progress"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "second ticket for shared blob gc test",
		AC:          []ticket.AcceptanceCriterion{{Description: "proof command works"}},
	}); err != nil {
		t.Fatalf("create second ticket: %v", err)
	}

	fixtureRel := filepath.Join("fixtures", "proof.png")
	fixtureAbs := filepath.Join(tmpDir, fixtureRel)
	writeProofPNGFixture(t, fixtureAbs)

	first, err := s.AddProof(context.Background(), proof.AddInput{
		TicketID:   "TKT-001",
		SourcePath: fixtureRel,
		ProofTitle: "A",
		Note:       "A",
		AddedAt:    "2026-03-16T20:00:00Z",
	})
	if err != nil {
		t.Fatalf("seed first proof: %v", err)
	}
	second, err := s.AddProof(context.Background(), proof.AddInput{
		TicketID:   "TKT-002",
		SourcePath: fixtureRel,
		ProofTitle: "B",
		Note:       "B",
		AddedAt:    "2026-03-16T20:01:00Z",
	})
	if err != nil {
		t.Fatalf("seed second proof: %v", err)
	}
	if first.File.Path != second.File.Path {
		t.Fatalf("expected shared blob path, got %q and %q", first.File.Path, second.File.Path)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(new(bytes.Buffer))

	rootCmd.SetArgs([]string{"proof", "remove", "TKT-001", "--proof-id", first.ID})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("remove first proof failed: %v", err)
	}

	out.Reset()
	rootCmd.SetArgs([]string{"proof", "gc"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("gc while still referenced failed: %v", err)
	}
	var gcPayload struct {
		GC struct {
			Removed int `json:"removed"`
		} `json:"gc"`
	}
	if err := json.Unmarshal(out.Bytes(), &gcPayload); err != nil {
		t.Fatalf("parse gc payload failed: %v; body=%s", err, out.String())
	}
	if gcPayload.GC.Removed != 0 {
		t.Fatalf("expected no removals while blob is referenced, got %+v", gcPayload)
	}

	out.Reset()
	rootCmd.SetArgs([]string{"proof", "remove", "TKT-002", "--proof-id", second.ID})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("remove second proof failed: %v", err)
	}

	out.Reset()
	rootCmd.SetArgs([]string{"proof", "gc"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("gc after unreference failed: %v", err)
	}
	gcPayload = struct {
		GC struct {
			Removed int `json:"removed"`
		} `json:"gc"`
	}{}
	if err := json.Unmarshal(out.Bytes(), &gcPayload); err != nil {
		t.Fatalf("parse gc payload failed: %v; body=%s", err, out.String())
	}
	if gcPayload.GC.Removed != 1 {
		t.Fatalf("expected exactly one blob removal, got %+v", gcPayload)
	}
}

func TestProofLifecycleEvents_AddRemoveOrderAndSchema(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	setupProofTicket(t, tmpDir)

	fixtureRel := filepath.Join("fixtures", "proof.png")
	fixtureAbs := filepath.Join(tmpDir, fixtureRel)
	writeProofPNGFixture(t, fixtureAbs)

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(new(bytes.Buffer))

	rootCmd.SetArgs([]string{"proof", "add", "TKT-001", "--file", fixtureRel, "--proof-title", "Before", "--note", "baseline"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("proof add failed: %v", err)
	}
	var addPayload struct {
		Proof struct {
			ID string `json:"id"`
		} `json:"proof"`
	}
	if err := json.Unmarshal(out.Bytes(), &addPayload); err != nil {
		t.Fatalf("parse add payload: %v", err)
	}

	out.Reset()
	rootCmd.SetArgs([]string{"proof", "remove", "TKT-001", "--proof-id", addPayload.Proof.ID})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("proof remove failed: %v", err)
	}

	events, err := lifecycle.Load(tmpDir)
	if err != nil {
		t.Fatalf("load lifecycle events: %v", err)
	}
	var proofEvents []lifecycle.Event
	for _, ev := range events {
		if ev.Type == lifecycle.EventProofMutation {
			proofEvents = append(proofEvents, ev)
		}
	}
	if len(proofEvents) != 2 {
		t.Fatalf("expected 2 proof mutation events, got %d (%+v)", len(proofEvents), proofEvents)
	}
	if proofEvents[0].Payload["action"] != "add" || proofEvents[1].Payload["action"] != "remove" {
		t.Fatalf("unexpected proof event action order: %+v", proofEvents)
	}
	for i, ev := range proofEvents {
		report := lifecycle.ValidateEvent(ev)
		if !report.Valid() {
			t.Fatalf("proof event %d failed schema validation: %+v", i, report.Errors)
		}
		for _, key := range []string{"ticket_id", "proof_id", "blob_sha256", "actor", "command"} {
			if value, ok := ev.Payload[key].(string); !ok || value == "" {
				t.Fatalf("proof event %d missing payload %s: %+v", i, key, ev.Payload)
			}
		}
	}
}
