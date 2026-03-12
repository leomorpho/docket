package cmd

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
)

type fileLock struct {
	TicketID     string   `json:"ticket_id"`
	WorktreePath string   `json:"worktree_path"`
	Files        []string `json:"files"`
	UpdatedAt    string   `json:"updated_at"`
}

type locksState struct {
	Locks []fileLock `json:"locks"`
}

func locksPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".docket", "locks.json")
}

func loadLocks(repoRoot string) (locksState, error) {
	var st locksState
	data, err := os.ReadFile(locksPath(repoRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return locksState{}, nil
		}
		return st, err
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, err
	}
	return st, nil
}

func saveLocks(repoRoot string, st locksState) error {
	if err := os.MkdirAll(filepath.Dir(locksPath(repoRoot)), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(locksPath(repoRoot), append(data, '\n'), 0o644)
}

func ensureLocksGitignored(repoRoot string) error {
	path := filepath.Join(repoRoot, ".gitignore")
	line := ".docket/locks.json"
	data, _ := os.ReadFile(path)
	content := string(data)
	if strings.Contains(content, line) {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if strings.TrimSpace(content) != "" {
		_, _ = f.WriteString("\n")
	}
	_, err = f.WriteString(line + "\n")
	return err
}

func claimFilesForWorktree(worktreePath string) []string {
	collect := func(args ...string) []string {
		c := exec.Command("git", append([]string{"-C", worktreePath}, args...)...)
		out, err := c.Output()
		if err != nil {
			return nil
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		files := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				files = append(files, line)
			}
		}
		return files
	}
	seen := map[string]bool{}
	files := []string{}
	for _, f := range append(collect("diff", "--name-only"), collect("diff", "--cached", "--name-only")...) {
		if seen[f] {
			continue
		}
		seen[f] = true
		files = append(files, f)
	}
	sort.Strings(files)
	return files
}

func upsertLock(repoRoot string, l fileLock) error {
	st, err := loadLocks(repoRoot)
	if err != nil {
		return err
	}
	next := make([]fileLock, 0, len(st.Locks)+1)
	replaced := false
	for _, existing := range st.Locks {
		if existing.TicketID == l.TicketID {
			next = append(next, l)
			replaced = true
			continue
		}
		next = append(next, existing)
	}
	if !replaced {
		next = append(next, l)
	}
	st.Locks = next
	return saveLocks(repoRoot, st)
}

func releaseLockForTicket(repoRoot, ticketID string) error {
	st, err := loadLocks(repoRoot)
	if err != nil {
		return err
	}
	next := make([]fileLock, 0, len(st.Locks))
	for _, l := range st.Locks {
		if l.TicketID == ticketID {
			continue
		}
		next = append(next, l)
	}
	st.Locks = next
	return saveLocks(repoRoot, st)
}

func activeInProgress(repoRoot string, ticketID string) bool {
	s := local.New(repoRoot)
	t, err := s.GetTicket(context.Background(), ticketID)
	if err != nil || t == nil {
		return false
	}
	return t.State == "in-progress"
}

func refreshLockClaims(repoRoot string) (locksState, error) {
	st, err := loadLocks(repoRoot)
	if err != nil {
		return st, err
	}
	for i := range st.Locks {
		files := claimFilesForWorktree(st.Locks[i].WorktreePath)
		if len(files) > 0 {
			st.Locks[i].Files = files
		}
		st.Locks[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return st, saveLocks(repoRoot, st)
}
