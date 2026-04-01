package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leomorpho/docket/internal/claim"
	"github.com/leomorpho/docket/internal/lifecycle"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestProofPipelineEndToEndIntegration(t *testing.T) {
	h := newFakeRepoHarness(t)
	ticketID := "TKT-944"
	h.seedTicket(ticketID, 944, ticket.State("ready"), []ticket.AcceptanceCriterion{{Description: "proof e2e"}, {Description: "proof cleanup"}})
	runGitSession(t, h.repo, "add", ".")
	runGitSession(t, h.repo, "commit", "-m", "seed proof ticket")

	if out, err := h.run("bootstrap", "--adapter", "codex"); err != nil {
		t.Fatalf("bootstrap failed: %v\n%s", err, out)
	}
	if out, err := h.run("update", ticketID, "--state", "running", "--allow-empty-startable-leaf"); err != nil {
		t.Fatalf("start ticket failed: %v\n%s", err, out)
	}

	cl, err := claim.GetClaim(h.repo, ticketID)
	if err != nil {
		t.Fatalf("read claim failed: %v", err)
	}
	if cl == nil {
		t.Fatalf("expected claim for %s after start", ticketID)
	}
	worktreeRepo := cl.Worktree

	proofRel := filepath.Join("fixtures", "proof.png")
	writeProofPNGFixture(t, filepath.Join(worktreeRepo, proofRel))

	mainRepo := h.repo
	h.repo = worktreeRepo
	defer func() { h.repo = mainRepo }()

	trace := []string{}
	runJSON := func(args ...string) (string, map[string]any) {
		withFormat := append(append([]string{}, args...), "--format", "json")
		out, err := h.run(withFormat...)
		if err != nil {
			t.Fatalf("%s failed: %v\n%s", strings.Join(withFormat, " "), err, out)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(out), &payload); err != nil {
			t.Fatalf("unmarshal %s json failed: %v\n%s", strings.Join(withFormat, " "), err, out)
		}
		trace = append(trace, "docket "+strings.Join(withFormat, " "))
		return out, payload
	}

	addOut, addPayload := runJSON(
		"proof", "add", ticketID,
		"--file", proofRel,
		"--proof-title", "Homepage baseline",
		"--note", "Capture baseline before UI change",
		"--captured-at", "2026-03-16T18:40:00Z",
	)
	proofObj, ok := addPayload["proof"].(map[string]any)
	if !ok {
		t.Fatalf("expected proof object in add payload: %+v", addPayload)
	}
	proofID, _ := proofObj["id"].(string)
	if strings.TrimSpace(proofID) == "" {
		t.Fatalf("expected non-empty proof id in add payload: %+v", addPayload)
	}

	metadataPath := filepath.Join(mainRepo, ".docket", "proofs", ticketID, "metadata.json")
	metadataRaw, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("read proof metadata fixture failed: %v", err)
	}

	showOut, showPayload := runJSON("show", ticketID)
	showProofs, ok := showPayload["proofs"].([]any)
	if !ok || len(showProofs) != 1 {
		t.Fatalf("expected one proof in show output, got %+v", showPayload["proofs"])
	}

	featurePath := filepath.Join(worktreeRepo, "src", "proof_feature.go")
	if err := os.MkdirAll(filepath.Dir(featurePath), 0o755); err != nil {
		t.Fatalf("mkdir feature path: %v", err)
	}
	if err := os.WriteFile(featurePath, []byte("package prooffeature\n\nfunc Marker() {}\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	runGitSession(t, worktreeRepo, "add", "src/proof_feature.go")
	runGitSession(t, worktreeRepo, "commit", "-m", "feat: proof context linkage", "-m", fmt.Sprintf("Ticket: %s", ticketID))

	h.repo = mainRepo
	if out, err := h.run("update", ticketID, "--state", "validated"); err != nil {
		t.Fatalf("finish ticket failed: %v\n%s", err, out)
	}
	h.repo = mainRepo

	scanOut, err := h.run("scan")
	if err != nil {
		t.Fatalf("scan failed: %v\n%s", err, scanOut)
	}
	trace = append(trace, "docket scan")

	contextOut, contextPayload := runJSON("context", "src/proof_feature.go")
	contextTickets, ok := contextPayload["tickets"].([]any)
	if !ok || len(contextTickets) == 0 {
		t.Fatalf("expected context tickets payload, got %+v", contextPayload)
	}
	firstTicket, ok := contextTickets[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected first context ticket payload: %+v", contextTickets[0])
	}
	contextProofs, ok := firstTicket["proofs"].([]any)
	if !ok || len(contextProofs) != 1 {
		t.Fatalf("expected one proof in context payload, got %+v", firstTicket["proofs"])
	}

	listOut, listPayload := runJSON("proof", "list", ticketID)
	listProofs, ok := listPayload["proofs"].([]any)
	if !ok || len(listProofs) != 1 {
		t.Fatalf("expected one proof in list output, got %+v", listPayload["proofs"])
	}

	removeOut, removePayload := runJSON("proof", "remove", ticketID, "--proof-id", proofID)
	removed, ok := removePayload["removed"].(map[string]any)
	if !ok || removed["id"] != proofID {
		t.Fatalf("expected removed proof %s, got %+v", proofID, removePayload)
	}

	gcOut, gcPayload := runJSON("proof", "gc")
	gc, ok := gcPayload["gc"].(map[string]any)
	if !ok {
		t.Fatalf("expected gc summary object, got %+v", gcPayload)
	}
	removedCount, ok := gc["removed"].(float64)
	if !ok || removedCount < 1 {
		t.Fatalf("expected gc removed count >= 1, got %+v", gcPayload)
	}

	listAfterOut, listAfterPayload := runJSON("proof", "list", ticketID)
	listAfterProofs, ok := listAfterPayload["proofs"].([]any)
	if !ok || len(listAfterProofs) != 0 {
		t.Fatalf("expected zero proofs after remove/gc, got %+v", listAfterPayload["proofs"])
	}

	events, err := lifecycle.Load(h.repo)
	if err != nil {
		t.Fatalf("load lifecycle events failed: %v", err)
	}
	proofEvents := make([]lifecycle.Event, 0, 2)
	for _, ev := range events {
		if ev.Type == lifecycle.EventProofMutation {
			proofEvents = append(proofEvents, ev)
		}
	}
	if len(proofEvents) != 2 {
		t.Fatalf("expected 2 proof mutation events, got %d (%+v)", len(proofEvents), proofEvents)
	}
	if proofEvents[0].Payload["action"] != "add" || proofEvents[1].Payload["action"] != "remove" {
		t.Fatalf("unexpected proof mutation event order: %+v", proofEvents)
	}
	eventLogRaw, err := os.ReadFile(lifecycle.LogPath(h.repo))
	if err != nil {
		t.Fatalf("read lifecycle log fixture failed: %v", err)
	}

	addFixture := h.writeFixture(filepath.Join("proof-pipeline", "proof-add.json"), []byte(addOut))
	showFixture := h.writeFixture(filepath.Join("proof-pipeline", "show.json"), []byte(showOut))
	contextFixture := h.writeFixture(filepath.Join("proof-pipeline", "context.json"), []byte(contextOut))
	listFixture := h.writeFixture(filepath.Join("proof-pipeline", "proof-list.json"), []byte(listOut))
	removeFixture := h.writeFixture(filepath.Join("proof-pipeline", "proof-remove.json"), []byte(removeOut))
	gcFixture := h.writeFixture(filepath.Join("proof-pipeline", "proof-gc.json"), []byte(gcOut))
	listAfterFixture := h.writeFixture(filepath.Join("proof-pipeline", "proof-list-after-remove.json"), []byte(listAfterOut))
	metadataFixture := h.writeFixture(filepath.Join("proof-pipeline", "proof-metadata.json"), metadataRaw)
	traceFixture := h.writeFixture(filepath.Join("proof-pipeline", "command-trace.txt"), []byte(strings.Join(trace, "\n")+"\n"))
	eventFixture := h.writeFixture(filepath.Join("proof-pipeline", "proof-events.jsonl"), eventLogRaw)
	t.Logf(
		"proof pipeline fixtures: %s | %s | %s | %s | %s | %s | %s | %s | %s | %s",
		addFixture,
		showFixture,
		contextFixture,
		listFixture,
		removeFixture,
		gcFixture,
		listAfterFixture,
		metadataFixture,
		traceFixture,
		eventFixture,
	)
}
