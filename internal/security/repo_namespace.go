package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

type RepoNamespaceStore struct {
	docketHome string
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
