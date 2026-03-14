package security

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEventLedgerAppendAndLoadChain(t *testing.T) {
	repo := t.TempDir()
	docketHome := t.TempDir()
	ks := NewFileKeystore(docketHome)
	if err := ks.Create("passphrase-123"); err != nil {
		t.Fatalf("create keystore failed: %v", err)
	}

	ledger := NewEventLedger(repo, ks, "dev_test")
	first, err := ledger.Append(LedgerAppendInput{
		Type:         EventWorkflowActivated,
		RepoID:       "drid_test",
		Actor:        "human:test",
		WorkflowHash: "wf-hash-1",
		Metadata: map[string]any{
			"source": "workflow.lock",
		},
	})
	if err != nil {
		t.Fatalf("append first event failed: %v", err)
	}
	if first.Hash == "" || first.Signature == "" {
		t.Fatalf("expected first event hash and signature, got %#v", first)
	}
	if first.PrevHash != "" {
		t.Fatalf("expected empty prev_hash for first event, got %q", first.PrevHash)
	}

	second, err := ledger.Append(LedgerAppendInput{
		Type:         EventRunStarted,
		RepoID:       "drid_test",
		TicketID:     "TKT-191",
		Actor:        "agent:test",
		Commit:       "abc123",
		Metadata:     map[string]any{"branch": "docket/TKT-191"},
		WorktreePath: filepath.Join(repo, "wt", "TKT-191"),
	})
	if err != nil {
		t.Fatalf("append second event failed: %v", err)
	}
	if second.PrevHash != first.Hash {
		t.Fatalf("expected second prev_hash=%q, got %q", first.Hash, second.PrevHash)
	}

	events, err := ledger.Load()
	if err != nil {
		t.Fatalf("load events failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[1].PrevHash != events[0].Hash {
		t.Fatalf("expected chain linkage, got first=%q second.prev=%q", events[0].Hash, events[1].PrevHash)
	}
}

func TestEventLedgerRejectsImpossiblePredecessor(t *testing.T) {
	repo := t.TempDir()
	docketHome := t.TempDir()
	ks := NewFileKeystore(docketHome)
	if err := ks.Create("passphrase-123"); err != nil {
		t.Fatalf("create keystore failed: %v", err)
	}
	ledger := NewEventLedger(repo, ks, "dev_test")

	if _, err := ledger.Append(LedgerAppendInput{
		Type:     EventRunStarted,
		RepoID:   "drid_test",
		TicketID: "TKT-191",
		Actor:    "agent:test",
	}); err != nil {
		t.Fatalf("append first event failed: %v", err)
	}
	if _, err := ledger.Append(LedgerAppendInput{
		Type:     EventRunStopped,
		RepoID:   "drid_test",
		TicketID: "TKT-191",
		Actor:    "agent:test",
	}); err != nil {
		t.Fatalf("append second event failed: %v", err)
	}

	path := LedgerPath(repo)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ledger failed: %v", err)
	}
	lines := bytesSplitLines(data)
	if len(lines) != 2 {
		t.Fatalf("expected 2 ledger lines, got %d", len(lines))
	}
	var ev LedgerEvent
	if err := json.Unmarshal(lines[1], &ev); err != nil {
		t.Fatalf("unmarshal second event failed: %v", err)
	}
	ev.PrevHash = "deadbeef"
	ev.Hash = ""
	ev.Signature = ""
	hashHex, hashBytes, err := hashLedgerEvent(ev)
	if err != nil {
		t.Fatalf("hash tampered event failed: %v", err)
	}
	sig, signErr := ks.SignDevice(hashBytes)
	if signErr != nil {
		t.Fatalf("sign tampered event failed: %v", signErr)
	}
	ev.Hash = hashHex
	ev.Signature = encodeB64(sig)
	updated, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal tampered event failed: %v", err)
	}
	repaired := append(append(lines[0], '\n'), append(updated, '\n')...)
	if err := os.WriteFile(path, repaired, 0o644); err != nil {
		t.Fatalf("write tampered ledger failed: %v", err)
	}

	if _, err := ledger.Load(); !errors.Is(err, ErrLedgerInvalidChain) {
		t.Fatalf("expected invalid chain error, got: %v", err)
	}
}

func TestEventLedgerRejectsMalformedEvent(t *testing.T) {
	repo := t.TempDir()
	path := LedgerPath(repo)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir ledger dir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("{\"version\":1,\"type\":\"run_started\"}\n"), 0o644); err != nil {
		t.Fatalf("write malformed ledger failed: %v", err)
	}

	ledger := NewEventLedger(repo, nil, "")
	if _, err := ledger.Load(); !errors.Is(err, ErrLedgerMalformed) {
		t.Fatalf("expected malformed event error, got: %v", err)
	}
}

func bytesSplitLines(data []byte) [][]byte {
	lines := [][]byte{}
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

func encodeB64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}
