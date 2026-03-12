package claim

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/leomorpho/docket/internal/git"
)

// ClaimMetadata represents the ownership of a ticket by an agent.
type ClaimMetadata struct {
	AgentID  string    `json:"agent_id"`
	Worktree string    `json:"worktree"`
	At       time.Time `json:"at"`
}

// GetClaimsDir returns the path to the claims directory in the shared .git folder.
func GetClaimsDir(repoRoot string) (string, error) {
	common, err := git.GetGitCommonDir(repoRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(common, "docket", "claims"), nil
}

// Claim attempts to claim a ticket for an agent in a specific worktree.
// It fails if the ticket is already claimed by someone else.
func Claim(repoRoot, ticketID, worktreePath, agentID string) error {
	dir, err := GetClaimsDir(repoRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path := filepath.Join(dir, ticketID+".json")

	m := ClaimMetadata{
		AgentID:  agentID,
		Worktree: worktreePath,
		At:       time.Now().UTC().Truncate(time.Second),
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			// Already exists, read who owns it for better error message
			if existing, readErr := GetClaim(repoRoot, ticketID); readErr == nil && existing != nil {
				if existing.AgentID == agentID && existing.Worktree == worktreePath {
					return nil // Already claimed by the same agent/worktree
				}
				return fmt.Errorf("ticket %s is already claimed by %s in %s", ticketID, existing.AgentID, existing.Worktree)
			}
			return fmt.Errorf("ticket %s is already claimed", ticketID)
		}
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

// Release removes a claim for a ticket.
func Release(repoRoot, ticketID string) error {
	dir, err := GetClaimsDir(repoRoot)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, ticketID+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// GetClaim returns the metadata for a claim, or nil if not claimed.
func GetClaim(repoRoot, ticketID string) (*ClaimMetadata, error) {
	dir, err := GetClaimsDir(repoRoot)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, ticketID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var m ClaimMetadata
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
