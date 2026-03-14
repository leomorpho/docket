package security

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const repoIDFile = "repo_id"

var repoIDPattern = regexp.MustCompile(`^drid_[0-9a-fA-F-]{36}$`)

type trustAnchorFile struct {
	RepoID    string `json:"repo_id"`
	SignerID  string `json:"signer_id"`
	UpdatedAt string `json:"updated_at"`
}

type trustedLedgerHeadFile struct {
	RepoID     string `json:"repo_id"`
	LedgerHead string `json:"ledger_head"`
	UpdatedAt  string `json:"updated_at"`
}

type workflowActivationFile struct {
	RepoID       string `json:"repo_id"`
	WorkflowHash string `json:"workflow_hash"`
	ActivatedAt  string `json:"activated_at"`
	ActivatedBy  string `json:"activated_by"`
}

type RunManifest struct {
	RepoID            string `json:"repo_id"`
	TicketID          string `json:"ticket_id"`
	Actor             string `json:"actor"`
	ActorType         string `json:"actor_type"`
	WorktreePath      string `json:"worktree_path"`
	Branch            string `json:"branch"`
	WorkflowHash      string `json:"workflow_hash"`
	StartedAt         string `json:"started_at"`
	RoutingTier       string `json:"routing_tier,omitempty"`
	RoutingProvider   string `json:"routing_provider,omitempty"`
	RoutingModelID    string `json:"routing_model_id,omitempty"`
	RoutingRationale  string `json:"routing_rationale,omitempty"`
	RoutingRecordedAt string `json:"routing_recorded_at,omitempty"`
}

var (
	ErrRunManifestMissing = errors.New("run manifest missing")
	ErrRunManifestStale   = errors.New("run manifest stale")
	ErrRunContextMismatch = errors.New("run context mismatch")
	ErrRunManifestInvalid = errors.New("run manifest invalid")
	ErrLedgerHeadRollback = errors.New("ledger head rollback detected")
	ErrLedgerHeadAnchor   = errors.New("ledger head trust anchor is malformed")
	DefaultRunManifestTTL = 24 * time.Hour
)

type RepoNamespaceStore struct {
	docketHome string
}

type ContextBinding struct {
	RepoID       string `json:"repo_id"`
	Actor        string `json:"actor"`
	TicketID     string `json:"ticket_id"`
	WorktreePath string `json:"worktree_path"`
	RunStartedAt string `json:"run_started_at"`
	UpdatedAt    string `json:"updated_at"`
}

func NewRepoNamespaceStore(docketHome string) *RepoNamespaceStore {
	return &RepoNamespaceStore{docketHome: docketHome}
}

func (s *RepoNamespaceStore) EnsureRepoNamespace(repoRoot string) (repoID string, namespaceDir string, err error) {
	repoID, err = GetOrCreateRepoID(repoRoot)
	if err != nil {
		return "", "", err
	}
	namespaceDir = s.repoNamespaceDir(repoID)
	if err := os.MkdirAll(namespaceDir, 0o755); err != nil {
		return "", "", err
	}
	return repoID, namespaceDir, nil
}

func (s *RepoNamespaceStore) SetTrustAnchor(repoRoot, signerID string) (string, error) {
	if signerID == "" {
		return "", fmt.Errorf("signer_id is required")
	}
	repoID, dir, err := s.EnsureRepoNamespace(repoRoot)
	if err != nil {
		return "", err
	}
	rec := trustAnchorFile{
		RepoID:    repoID,
		SignerID:  signerID,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "trust_anchor.json"), append(data, '\n'), 0o600); err != nil {
		return "", err
	}
	return repoID, nil
}

func (s *RepoNamespaceStore) GetTrustAnchor(repoRoot string) (repoID string, signerID string, ok bool, err error) {
	repoID, err = GetOrCreateRepoID(repoRoot)
	if err != nil {
		return "", "", false, err
	}
	return s.GetTrustAnchorByRepoID(repoID)
}

func (s *RepoNamespaceStore) GetTrustAnchorByRepoID(repoID string) (string, string, bool, error) {
	path := filepath.Join(s.repoNamespaceDir(repoID), "trust_anchor.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return repoID, "", false, nil
		}
		return "", "", false, err
	}
	var rec trustAnchorFile
	if err := json.Unmarshal(data, &rec); err != nil {
		return "", "", false, fmt.Errorf("%w: invalid trust anchor JSON", ErrKeystoreMalformed)
	}
	if rec.RepoID == "" || rec.SignerID == "" {
		return "", "", false, fmt.Errorf("%w: trust anchor missing repo_id or signer_id", ErrKeystoreMalformed)
	}
	return rec.RepoID, rec.SignerID, true, nil
}

func (s *RepoNamespaceStore) SetTrustedLedgerHead(repoRoot, headHash string) (string, error) {
	repoID, dir, err := s.EnsureRepoNamespace(repoRoot)
	if err != nil {
		return "", err
	}
	rec := trustedLedgerHeadFile{
		RepoID:     repoID,
		LedgerHead: strings.TrimSpace(headHash),
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "trusted_ledger_head.json"), append(data, '\n'), 0o600); err != nil {
		return "", err
	}
	return repoID, nil
}

func (s *RepoNamespaceStore) GetTrustedLedgerHead(repoRoot string) (repoID string, headHash string, ok bool, err error) {
	repoID, err = GetOrCreateRepoID(repoRoot)
	if err != nil {
		return "", "", false, err
	}
	path := filepath.Join(s.repoNamespaceDir(repoID), "trusted_ledger_head.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return repoID, "", false, nil
		}
		return "", "", false, err
	}
	var rec trustedLedgerHeadFile
	if err := json.Unmarshal(data, &rec); err != nil {
		return "", "", false, fmt.Errorf("%w: invalid trusted ledger head JSON", ErrLedgerHeadAnchor)
	}
	if rec.RepoID == "" {
		return "", "", false, fmt.Errorf("%w: trusted ledger head missing repo_id", ErrLedgerHeadAnchor)
	}
	if rec.RepoID != repoID {
		return "", "", false, fmt.Errorf("%w: trusted ledger head repo_id mismatch", ErrLedgerHeadAnchor)
	}
	return rec.RepoID, strings.TrimSpace(rec.LedgerHead), true, nil
}

func (s *RepoNamespaceStore) VerifyAndAdvanceTrustedLedgerHead(repoRoot string) error {
	events, err := NewEventLedger(repoRoot, nil, "").Load()
	if err != nil {
		return fmt.Errorf("loading ledger: %w", err)
	}
	currentHead := ""
	if len(events) > 0 {
		currentHead = strings.TrimSpace(events[len(events)-1].Hash)
	}

	repoID, trustedHead, ok, err := s.GetTrustedLedgerHead(repoRoot)
	if err != nil {
		return err
	}
	if !ok {
		_, err := s.SetTrustedLedgerHead(repoRoot, currentHead)
		return err
	}
	if trustedHead == currentHead {
		return nil
	}
	if trustedHead == "" {
		_, err := s.SetTrustedLedgerHead(repoRoot, currentHead)
		return err
	}

	for _, ev := range events {
		if strings.TrimSpace(ev.Hash) == trustedHead {
			_, err := s.SetTrustedLedgerHead(repoRoot, currentHead)
			return err
		}
	}

	return fmt.Errorf(
		"%w: repo %s head %s does not descend from trusted head %s. Repair by restoring the newer ledger or explicitly resetting %s only if you intend to trust this state",
		ErrLedgerHeadRollback,
		repoID,
		shortHash(currentHead),
		shortHash(trustedHead),
		filepath.Join(s.repoNamespaceDir(repoID), "trusted_ledger_head.json"),
	)
}

func (s *RepoNamespaceStore) SetActiveWorkflowHash(repoRoot, workflowHash, actor string) (string, error) {
	if workflowHash == "" {
		return "", fmt.Errorf("workflow hash is required")
	}
	repoID, dir, err := s.EnsureRepoNamespace(repoRoot)
	if err != nil {
		return "", err
	}
	rec := workflowActivationFile{
		RepoID:       repoID,
		WorkflowHash: workflowHash,
		ActivatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		ActivatedBy:  actor,
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "workflow_activation.json"), append(data, '\n'), 0o600); err != nil {
		return "", err
	}
	return repoID, nil
}

func (s *RepoNamespaceStore) GetActiveWorkflowHash(repoRoot string) (string, bool, error) {
	repoID, err := GetOrCreateRepoID(repoRoot)
	if err != nil {
		return "", false, err
	}
	path := filepath.Join(s.repoNamespaceDir(repoID), "workflow_activation.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	var rec workflowActivationFile
	if err := json.Unmarshal(data, &rec); err != nil {
		return "", false, fmt.Errorf("%w: invalid workflow activation JSON", ErrKeystoreMalformed)
	}
	if rec.WorkflowHash == "" {
		return "", false, fmt.Errorf("%w: workflow activation missing hash", ErrKeystoreMalformed)
	}
	return rec.WorkflowHash, true, nil
}

func actorType(actor string) string {
	switch {
	case strings.HasPrefix(actor, "agent:"):
		return "agent"
	case strings.HasPrefix(actor, "human:"):
		return "human"
	default:
		return "unknown"
	}
}

func (s *RepoNamespaceStore) RecordRunStart(repoRoot, ticketID, actor, worktreePath, branch, workflowHash string) error {
	if ticketID == "" {
		return fmt.Errorf("ticket ID is required")
	}
	if actor == "" {
		return fmt.Errorf("actor is required")
	}
	if worktreePath == "" {
		return fmt.Errorf("worktree path is required")
	}
	if branch == "" {
		return fmt.Errorf("branch is required")
	}
	repoID, dir, err := s.EnsureRepoNamespace(repoRoot)
	if err != nil {
		return err
	}
	rec := RunManifest{
		RepoID:       repoID,
		TicketID:     ticketID,
		Actor:        actor,
		ActorType:    actorType(actor),
		WorktreePath: worktreePath,
		Branch:       branch,
		WorkflowHash: workflowHash,
		StartedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	runsDir := filepath.Join(dir, "runs")
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(runsDir, ticketID+".json"), append(data, '\n'), 0o600)
}

func (s *RepoNamespaceStore) RecordRunRoutingDecision(repoRoot, ticketID, tier, provider, modelID, rationale string) error {
	rec, ok, err := s.GetRunManifest(repoRoot, ticketID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: %s", ErrRunManifestMissing, ticketID)
	}
	rec.RoutingTier = strings.TrimSpace(tier)
	rec.RoutingProvider = strings.TrimSpace(provider)
	rec.RoutingModelID = strings.TrimSpace(modelID)
	rec.RoutingRationale = strings.TrimSpace(rationale)
	rec.RoutingRecordedAt = time.Now().UTC().Format(time.RFC3339Nano)

	repoID, dir, err := s.EnsureRepoNamespace(repoRoot)
	if err != nil {
		return err
	}
	rec.RepoID = repoID
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	runsDir := filepath.Join(dir, "runs")
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(runsDir, ticketID+".json"), append(data, '\n'), 0o600)
}

func (s *RepoNamespaceStore) GetRunManifest(repoRoot, ticketID string) (RunManifest, bool, error) {
	repoID, err := GetOrCreateRepoID(repoRoot)
	if err != nil {
		return RunManifest{}, false, err
	}
	path := filepath.Join(s.repoNamespaceDir(repoID), "runs", ticketID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return RunManifest{}, false, nil
		}
		return RunManifest{}, false, err
	}
	var rec RunManifest
	if err := json.Unmarshal(data, &rec); err != nil {
		return RunManifest{}, false, err
	}
	if err := validateRunManifest(repoID, ticketID, rec); err != nil {
		return RunManifest{}, false, err
	}
	return rec, true, nil
}

func validateRunManifest(expectedRepoID, expectedTicketID string, rec RunManifest) error {
	if strings.TrimSpace(rec.RepoID) == "" {
		return fmt.Errorf("%w: repo_id is required", ErrRunManifestInvalid)
	}
	if rec.RepoID != expectedRepoID {
		return fmt.Errorf("%w: repo_id mismatch (expected %s, got %s)", ErrRunManifestInvalid, expectedRepoID, rec.RepoID)
	}
	if strings.TrimSpace(rec.TicketID) == "" {
		return fmt.Errorf("%w: ticket_id is required", ErrRunManifestInvalid)
	}
	if rec.TicketID != expectedTicketID {
		return fmt.Errorf("%w: ticket_id mismatch (expected %s, got %s)", ErrRunManifestInvalid, expectedTicketID, rec.TicketID)
	}
	if strings.TrimSpace(rec.Actor) == "" {
		return fmt.Errorf("%w: actor is required", ErrRunManifestInvalid)
	}
	if strings.TrimSpace(rec.ActorType) == "" {
		return fmt.Errorf("%w: actor_type is required", ErrRunManifestInvalid)
	}
	if strings.TrimSpace(rec.WorktreePath) == "" {
		return fmt.Errorf("%w: worktree_path is required", ErrRunManifestInvalid)
	}
	if strings.TrimSpace(rec.Branch) == "" {
		return fmt.Errorf("%w: branch is required", ErrRunManifestInvalid)
	}
	if strings.TrimSpace(rec.StartedAt) == "" {
		return fmt.Errorf("%w: started_at is required", ErrRunManifestInvalid)
	}
	return nil
}

func (s *RepoNamespaceStore) VerifyRunContext(repoRoot, ticketID, actor, worktreePath, branch, workflowHash string) error {
	rec, ok, err := s.GetRunManifest(repoRoot, ticketID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: %s", ErrRunManifestMissing, ticketID)
	}

	startedAt, err := time.Parse(time.RFC3339Nano, rec.StartedAt)
	if err != nil {
		return fmt.Errorf("%w: invalid started_at for %s", ErrRunManifestStale, ticketID)
	}
	if time.Since(startedAt) > DefaultRunManifestTTL {
		return fmt.Errorf("%w: %s started at %s", ErrRunManifestStale, ticketID, rec.StartedAt)
	}

	if actor != "" && rec.Actor != actor {
		return fmt.Errorf("%w: actor mismatch (expected %s, got %s)", ErrRunContextMismatch, rec.Actor, actor)
	}
	if worktreePath != "" {
		expAbs, _ := filepath.Abs(rec.WorktreePath)
		gotAbs, _ := filepath.Abs(worktreePath)
		if expAbs != gotAbs {
			return fmt.Errorf("%w: worktree mismatch (expected %s, got %s)", ErrRunContextMismatch, rec.WorktreePath, worktreePath)
		}
	}
	if branch != "" && rec.Branch != branch {
		return fmt.Errorf("%w: branch mismatch (expected %s, got %s)", ErrRunContextMismatch, rec.Branch, branch)
	}
	if workflowHash != "" && rec.WorkflowHash != workflowHash {
		return fmt.Errorf("%w: workflow hash mismatch (expected %s, got %s)", ErrRunContextMismatch, rec.WorkflowHash, workflowHash)
	}
	return nil
}

func HashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:]), nil
}

func GetOrCreateRepoID(repoRoot string) (string, error) {
	if repoRoot == "" {
		return "", fmt.Errorf("repo root is required")
	}
	path := filepath.Join(repoRoot, ".docket", repoIDFile)
	if data, err := os.ReadFile(path); err == nil {
		id := string(trimSpace(data))
		if !repoIDPattern.MatchString(id) {
			return "", fmt.Errorf("invalid repo ID format at %s", path)
		}
		return id, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	id := "drid_" + uuid.NewString()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(id+"\n"), 0o644); err != nil {
		return "", err
	}
	return id, nil
}

func (s *RepoNamespaceStore) repoNamespaceDir(repoID string) string {
	return filepath.Join(s.docketHome, "repos", repoID)
}

func (s *RepoNamespaceStore) UpdateContextBinding(repoRoot, actor, ticketID, worktreePath, runStartedAt string) (bool, string, error) {
	if actor == "" || ticketID == "" || worktreePath == "" {
		return false, "", fmt.Errorf("actor, ticketID, and worktreePath are required")
	}
	repoID, dir, err := s.EnsureRepoNamespace(repoRoot)
	if err != nil {
		return false, "", err
	}
	contextDir := filepath.Join(dir, "context")
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		return false, "", err
	}
	actorSum := sha256.Sum256([]byte(actor))
	path := filepath.Join(contextDir, fmt.Sprintf("%x.json", actorSum[:8]))

	reset := true
	reason := "new_context"
	if data, readErr := os.ReadFile(path); readErr == nil {
		var prev ContextBinding
		if unmarshalErr := json.Unmarshal(data, &prev); unmarshalErr == nil {
			switch {
			case prev.TicketID != ticketID:
				reason = "ticket_changed"
			case prev.WorktreePath != worktreePath:
				reason = "worktree_changed"
			case prev.RunStartedAt != "" && runStartedAt != "" && prev.RunStartedAt != runStartedAt:
				reason = "run_changed"
			default:
				reset = false
				reason = ""
			}
		}
	}

	next := ContextBinding{
		RepoID:       repoID,
		Actor:        actor,
		TicketID:     ticketID,
		WorktreePath: worktreePath,
		RunStartedAt: runStartedAt,
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	b, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return false, "", err
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		return false, "", err
	}
	return reset, reason, nil
}

func trimSpace(b []byte) []byte {
	start := 0
	end := len(b)
	for start < end && (b[start] == ' ' || b[start] == '\n' || b[start] == '\r' || b[start] == '\t') {
		start++
	}
	for end > start && (b[end-1] == ' ' || b[end-1] == '\n' || b[end-1] == '\r' || b[end-1] == '\t') {
		end--
	}
	return b[start:end]
}

func shortHash(hash string) string {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return "<empty>"
	}
	if len(hash) <= 12 {
		return hash
	}
	return hash[:12]
}
