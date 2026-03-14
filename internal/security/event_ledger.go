package security

import (
	"bufio"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const EventLedgerVersion = 1

var (
	ErrLedgerMalformed    = errors.New("ledger event malformed")
	ErrLedgerInvalidChain = errors.New("ledger chain is invalid")
)

type LedgerEventType string

const (
	EventWorkflowActivated           LedgerEventType = "workflow_activated"
	EventPrivilegedTransitionApprove LedgerEventType = "privileged_transition_approved"
	EventRunStarted                  LedgerEventType = "run_started"
	EventRunStopped                  LedgerEventType = "run_stopped"
	EventCommitLinked                LedgerEventType = "commit_linked"
)

type LedgerEvent struct {
	Version         int             `json:"version"`
	Type            LedgerEventType `json:"type"`
	RepoID          string          `json:"repo_id"`
	TicketID        string          `json:"ticket_id,omitempty"`
	Actor           string          `json:"actor"`
	OccurredAt      string          `json:"occurred_at"`
	WorkflowHash    string          `json:"workflow_hash,omitempty"`
	Commit          string          `json:"commit,omitempty"`
	WorktreePath    string          `json:"worktree_path,omitempty"`
	Metadata        map[string]any  `json:"metadata,omitempty"`
	PrevHash        string          `json:"prev_hash,omitempty"`
	Hash            string          `json:"hash"`
	SignerID        string          `json:"signer_id"`
	SignerPublicKey string          `json:"signer_public_key"`
	Signature       string          `json:"signature"`
}

type LedgerAppendInput struct {
	Type         LedgerEventType
	RepoID       string
	TicketID     string
	Actor        string
	WorkflowHash string
	Commit       string
	WorktreePath string
	Metadata     map[string]any
}

type EventLedger struct {
	repoRoot string
	keystore KeystoreProvider
	signerID string
}

func NewEventLedger(repoRoot string, keystore KeystoreProvider, signerID string) *EventLedger {
	return &EventLedger{
		repoRoot: repoRoot,
		keystore: keystore,
		signerID: signerID,
	}
}

func LedgerPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".docket", "ledger", "events.jsonl")
}

func (l *EventLedger) Append(input LedgerAppendInput) (LedgerEvent, error) {
	if l.keystore == nil {
		return LedgerEvent{}, fmt.Errorf("keystore is required")
	}
	if strings.TrimSpace(l.signerID) == "" {
		return LedgerEvent{}, fmt.Errorf("signer ID is required")
	}
	if strings.TrimSpace(input.RepoID) == "" {
		return LedgerEvent{}, fmt.Errorf("repo ID is required")
	}
	if strings.TrimSpace(input.Actor) == "" {
		return LedgerEvent{}, fmt.Errorf("actor is required")
	}
	if !isValidLedgerEventType(input.Type) {
		return LedgerEvent{}, fmt.Errorf("%w: unsupported event type %q", ErrLedgerMalformed, input.Type)
	}

	events, err := l.Load()
	if err != nil {
		return LedgerEvent{}, err
	}
	prevHash := ""
	if len(events) > 0 {
		prevHash = events[len(events)-1].Hash
	}
	pub, err := l.keystore.DevicePublicKey()
	if err != nil {
		return LedgerEvent{}, err
	}

	event := LedgerEvent{
		Version:         EventLedgerVersion,
		Type:            input.Type,
		RepoID:          input.RepoID,
		TicketID:        strings.TrimSpace(input.TicketID),
		Actor:           strings.TrimSpace(input.Actor),
		OccurredAt:      time.Now().UTC().Format(time.RFC3339Nano),
		WorkflowHash:    strings.TrimSpace(input.WorkflowHash),
		Commit:          strings.TrimSpace(input.Commit),
		WorktreePath:    strings.TrimSpace(input.WorktreePath),
		Metadata:        input.Metadata,
		PrevHash:        prevHash,
		SignerID:        l.signerID,
		SignerPublicKey: base64.StdEncoding.EncodeToString(pub),
	}
	hashHex, hashBytes, err := hashLedgerEvent(event)
	if err != nil {
		return LedgerEvent{}, err
	}
	event.Hash = hashHex
	sig, err := l.keystore.SignDevice(hashBytes)
	if err != nil {
		return LedgerEvent{}, err
	}
	event.Signature = base64.StdEncoding.EncodeToString(sig)

	if err := appendLedgerEventLine(LedgerPath(l.repoRoot), event); err != nil {
		return LedgerEvent{}, err
	}
	return event, nil
}

func (l *EventLedger) Load() ([]LedgerEvent, error) {
	path := LedgerPath(l.repoRoot)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	events := []LedgerEvent{}
	scanner := bufio.NewScanner(f)
	lineNo := 0
	prevHash := ""
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var ev LedgerEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, fmt.Errorf("%w: line %d is not valid JSON", ErrLedgerMalformed, lineNo)
		}
		if err := validateLedgerEvent(ev, prevHash); err != nil {
			return nil, err
		}
		prevHash = ev.Hash
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

func validateLedgerEvent(ev LedgerEvent, expectedPrevHash string) error {
	if ev.Version != EventLedgerVersion {
		return fmt.Errorf("%w: unsupported version %d", ErrLedgerMalformed, ev.Version)
	}
	if !isValidLedgerEventType(ev.Type) {
		return fmt.Errorf("%w: unsupported event type %q", ErrLedgerMalformed, ev.Type)
	}
	if strings.TrimSpace(ev.RepoID) == "" {
		return fmt.Errorf("%w: repo_id is required", ErrLedgerMalformed)
	}
	if strings.TrimSpace(ev.Actor) == "" {
		return fmt.Errorf("%w: actor is required", ErrLedgerMalformed)
	}
	if strings.TrimSpace(ev.OccurredAt) == "" {
		return fmt.Errorf("%w: occurred_at is required", ErrLedgerMalformed)
	}
	if _, err := time.Parse(time.RFC3339Nano, ev.OccurredAt); err != nil {
		return fmt.Errorf("%w: occurred_at must be RFC3339Nano", ErrLedgerMalformed)
	}
	if strings.TrimSpace(ev.SignerID) == "" {
		return fmt.Errorf("%w: signer_id is required", ErrLedgerMalformed)
	}
	if strings.TrimSpace(ev.SignerPublicKey) == "" {
		return fmt.Errorf("%w: signer_public_key is required", ErrLedgerMalformed)
	}
	if strings.TrimSpace(ev.Signature) == "" {
		return fmt.Errorf("%w: signature is required", ErrLedgerMalformed)
	}
	if strings.TrimSpace(ev.Hash) == "" {
		return fmt.Errorf("%w: hash is required", ErrLedgerMalformed)
	}

	pubBytes, err := base64.StdEncoding.DecodeString(ev.SignerPublicKey)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: signer_public_key is invalid", ErrLedgerMalformed)
	}
	sigBytes, err := base64.StdEncoding.DecodeString(ev.Signature)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return fmt.Errorf("%w: signature is invalid", ErrLedgerMalformed)
	}
	hashHex, hashBytes, err := hashLedgerEvent(ev)
	if err != nil {
		return err
	}
	if hashHex != ev.Hash {
		return fmt.Errorf("%w: hash mismatch", ErrLedgerMalformed)
	}
	if !ed25519.Verify(ed25519.PublicKey(pubBytes), hashBytes, sigBytes) {
		return fmt.Errorf("%w: signature verification failed", ErrLedgerMalformed)
	}
	if strings.TrimSpace(ev.PrevHash) != strings.TrimSpace(expectedPrevHash) {
		return fmt.Errorf("%w: impossible predecessor reference", ErrLedgerInvalidChain)
	}
	return nil
}

type ledgerHashPayload struct {
	Version      int             `json:"version"`
	Type         LedgerEventType `json:"type"`
	RepoID       string          `json:"repo_id"`
	TicketID     string          `json:"ticket_id,omitempty"`
	Actor        string          `json:"actor"`
	OccurredAt   string          `json:"occurred_at"`
	WorkflowHash string          `json:"workflow_hash,omitempty"`
	Commit       string          `json:"commit,omitempty"`
	WorktreePath string          `json:"worktree_path,omitempty"`
	Metadata     map[string]any  `json:"metadata,omitempty"`
	PrevHash     string          `json:"prev_hash,omitempty"`
	SignerID     string          `json:"signer_id"`
}

func hashLedgerEvent(ev LedgerEvent) (string, []byte, error) {
	payload := ledgerHashPayload{
		Version:      ev.Version,
		Type:         ev.Type,
		RepoID:       ev.RepoID,
		TicketID:     ev.TicketID,
		Actor:        ev.Actor,
		OccurredAt:   ev.OccurredAt,
		WorkflowHash: ev.WorkflowHash,
		Commit:       ev.Commit,
		WorktreePath: ev.WorktreePath,
		Metadata:     ev.Metadata,
		PrevHash:     ev.PrevHash,
		SignerID:     ev.SignerID,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", nil, fmt.Errorf("%w: cannot serialize hash payload", ErrLedgerMalformed)
	}
	sum := sha256.Sum256(data)
	hashHex := hex.EncodeToString(sum[:])
	hashBytes, err := hex.DecodeString(hashHex)
	if err != nil {
		return "", nil, err
	}
	return hashHex, hashBytes, nil
}

func appendLedgerEventLine(path string, event LedgerEvent) error {
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

func isValidLedgerEventType(eventType LedgerEventType) bool {
	switch eventType {
	case EventWorkflowActivated, EventPrivilegedTransitionApprove, EventRunStarted, EventRunStopped, EventCommitLinked:
		return true
	default:
		return false
	}
}
