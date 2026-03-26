package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/leomorpho/docket/internal/artifacts"
)

func repoLocalGitignoreLines() []string {
	lines := append([]string{}, artifacts.RepoLocalIgnorePatterns()...)
	lines = append(lines, artifacts.CanonicalLocalRootRelPath()+"/")
	sort.Strings(lines)
	return append([]string{"# docket"}, uniqueStrings(lines)...)
}

func ensureLocalArtifactsGitignored(repoRoot string) error {
	_, err := reconcileGitignoreFile(filepath.Join(repoRoot, ".gitignore"), repoLocalGitignoreLines())
	if err != nil {
		return err
	}
	return untrackManagedLocalArtifacts(repoRoot)
}

func reconcileGitignoreFile(path string, required []string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}

	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	required = uniqueStrings(required)
	requiredSet := make(map[string]struct{}, len(required))
	for _, line := range required {
		requiredSet[line] = struct{}{}
	}

	filtered := make([]string, 0, len(lines)+len(required))
	seenRequired := make(map[string]struct{}, len(required))
	changed := false
	for _, line := range lines {
		if _, ok := requiredSet[line]; ok {
			if _, seen := seenRequired[line]; seen {
				changed = true
				continue
			}
			seenRequired[line] = struct{}{}
		}
		filtered = append(filtered, line)
	}

	for _, line := range required {
		if _, seen := seenRequired[line]; seen {
			continue
		}
		filtered = append(filtered, line)
		changed = true
	}

	next := strings.Join(filtered, "\n")
	if next != "" {
		next += "\n"
	}
	if !changed && next == content {
		return false, nil
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func untrackManagedLocalArtifacts(repoRoot string) error {
	if !isGitWorkTree(repoRoot) {
		return nil
	}
	pathspecs := managedGitignorePathspecs()
	if len(pathspecs) == 0 {
		return nil
	}

	args := append([]string{"-C", repoRoot, "rm", "-r", "--cached", "--ignore-unmatch", "--quiet", "--"}, pathspecs...)
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("untracking local-only artifacts: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func managedGitignorePathspecs() []string {
	lines := repoLocalGitignoreLines()
	pathspecs := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		pathspecs = append(pathspecs, line)
	}
	return pathspecs
}

func isGitWorkTree(repoRoot string) bool {
	out, err := exec.Command("git", "-C", repoRoot, "rev-parse", "--is-inside-work-tree").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}
