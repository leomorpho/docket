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

type checkpoint struct {
	TicketID     string   `json:"ticket_id"`
	CreatedAt    string   `json:"created_at"`
	ACDone       int      `json:"ac_done"`
	ACTotal      int      `json:"ac_total"`
	ChangedFiles []string `json:"changed_files"`
	LastComments []string `json:"last_comments"`
	Branch       string   `json:"branch"`
	WorktreePath string   `json:"worktree_path"`
	Summary      string   `json:"summary,omitempty"`
}

func checkpointsDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".docket", "checkpoints")
}

func checkpointFileName(ticketID string) string {
	return ticketID + "-" + time.Now().UTC().Format("20060102T150405Z") + ".json"
}

func writeCheckpoint(repoRoot string, cp checkpoint) (string, error) {
	if err := os.MkdirAll(checkpointsDir(repoRoot), 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(checkpointsDir(repoRoot), checkpointFileName(cp.TicketID))
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func listCheckpointPaths(repoRoot, ticketID string) ([]string, error) {
	entries, err := os.ReadDir(checkpointsDir(repoRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	prefix := ticketID + "-"
	out := []string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		out = append(out, filepath.Join(checkpointsDir(repoRoot), e.Name()))
	}
	sort.Strings(out)
	return out, nil
}

func buildCheckpoint(repoRoot, ticketID string, summary string) checkpoint {
	s := local.New(repoRoot)
	t, _ := s.GetTicket(context.Background(), ticketID)
	done := 0
	total := 0
	lastComments := []string{}
	if t != nil {
		total = len(t.AC)
		for _, ac := range t.AC {
			if ac.Done {
				done++
			}
		}
		if len(t.Comments) > 0 {
			start := len(t.Comments) - 3
			if start < 0 {
				start = 0
			}
			for _, c := range t.Comments[start:] {
				lastComments = append(lastComments, strings.TrimSpace(c.Body))
			}
		}
	}
	return checkpoint{
		TicketID:     ticketID,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		ACDone:       done,
		ACTotal:      total,
		ChangedFiles: gitChangedFiles(repoRoot),
		LastComments: lastComments,
		Branch:       gitCurrentBranch(repoRoot),
		WorktreePath: repoRoot,
		Summary:      summary,
	}
}

func gitChangedFiles(repoRoot string) []string {
	c := exec.Command("git", "-C", repoRoot, "diff", "--name-only")
	out, err := c.Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	files := []string{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

func gitCurrentBranch(repoRoot string) string {
	c := exec.Command("git", "-C", repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := c.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
