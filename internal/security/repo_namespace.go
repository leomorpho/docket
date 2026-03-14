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
	if workflowHash == "" {
		return fmt.Errorf("workflow hash is required")
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
	return rec, true, nil
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
