package runstate

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/leomorpho/docket/internal/artifacts"
	docketgit "github.com/leomorpho/docket/internal/git"
)

const repoIDFile = "repo_id"

var repoIDPattern = regexp.MustCompile(`^drid_[0-9a-fA-F-]{36}$`)

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
	DefaultRunManifestTTL = 24 * time.Hour
)

type Store struct {
	root string
}

type ContextBinding struct {
	RepoID       string `json:"repo_id"`
	Actor        string `json:"actor"`
	TicketID     string `json:"ticket_id"`
	WorktreePath string `json:"worktree_path"`
	RunStartedAt string `json:"run_started_at"`
	UpdatedAt    string `json:"updated_at"`
}

func New(root string) *Store {
	return &Store{root: root}
}

func (s *Store) EnsureRepoNamespace(repoRoot string) (repoID string, namespaceDir string, err error) {
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

func (s *Store) SetActiveWorkflowHash(repoRoot, workflowHash, actor string) (string, error) {
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

func (s *Store) GetActiveWorkflowHash(repoRoot string) (string, bool, error) {
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
		return "", false, fmt.Errorf("invalid workflow activation JSON: %w", err)
	}
	if rec.WorkflowHash == "" {
		return "", false, fmt.Errorf("workflow activation missing hash")
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

func (s *Store) RecordRunStart(repoRoot, ticketID, actor, worktreePath, branch, workflowHash string) error {
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

func (s *Store) RecordRunRoutingDecision(repoRoot, ticketID, tier, provider, modelID, rationale string) error {
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

func (s *Store) GetRunManifest(repoRoot, ticketID string) (RunManifest, bool, error) {
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

func (s *Store) DeleteRunManifest(repoRoot, ticketID string) error {
	if strings.TrimSpace(ticketID) == "" {
		return fmt.Errorf("ticket ID is required")
	}
	repoID, err := GetOrCreateRepoID(repoRoot)
	if err != nil {
		return err
	}
	path := filepath.Join(s.repoNamespaceDir(repoID), "runs", ticketID+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Store) ListRunManifests() ([]RunManifest, error) {
	if strings.TrimSpace(s.root) == "" {
		return nil, nil
	}
	repoEntries, err := os.ReadDir(artifacts.HomePath(s.root, artifacts.HomeReposDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	manifests := make([]RunManifest, 0)
	for _, repoEntry := range repoEntries {
		if !repoEntry.IsDir() || !repoIDPattern.MatchString(repoEntry.Name()) {
			continue
		}
		runsDir := filepath.Join(artifacts.HomePath(s.root, artifacts.HomeReposDir), repoEntry.Name(), "runs")
		runEntries, err := os.ReadDir(runsDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, runEntry := range runEntries {
			if runEntry.IsDir() || !strings.HasSuffix(runEntry.Name(), ".json") {
				continue
			}
			ticketID := strings.TrimSuffix(runEntry.Name(), ".json")
			path := filepath.Join(runsDir, runEntry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			var rec RunManifest
			if err := json.Unmarshal(data, &rec); err != nil {
				return nil, err
			}
			if err := validateRunManifest(repoEntry.Name(), ticketID, rec); err != nil {
				return nil, err
			}
			manifests = append(manifests, rec)
		}
	}
	sort.Slice(manifests, func(i, j int) bool {
		if manifests[i].TicketID != manifests[j].TicketID {
			return manifests[i].TicketID < manifests[j].TicketID
		}
		if manifests[i].StartedAt != manifests[j].StartedAt {
			return manifests[i].StartedAt < manifests[j].StartedAt
		}
		return manifests[i].RepoID < manifests[j].RepoID
	})
	return manifests, nil
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

func (s *Store) VerifyRunContext(repoRoot, ticketID, actor, worktreePath, branch, workflowHash string) error {
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

func GetOrCreateRepoID(repoRoot string) (string, error) {
	if repoRoot == "" {
		return "", fmt.Errorf("repo root is required")
	}
	if commonDir, err := docketgit.GetGitCommonDir(repoRoot); err == nil && strings.TrimSpace(commonDir) != "" {
		repoRoot = filepath.Dir(commonDir)
	}
	path := artifacts.RepoPath(repoRoot, artifacts.RepoRepoID)
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

func (s *Store) repoNamespaceDir(repoID string) string {
	return artifacts.HomePath(s.root, artifacts.HomeReposDir, repoID)
}

func (s *Store) UpdateContextBinding(repoRoot, actor, ticketID, worktreePath, runStartedAt string) (bool, string, error) {
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
