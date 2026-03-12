package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	docketBlockStart = "<!-- docket:start -->"
	docketBlockEnd   = "<!-- docket:end -->"
)

type installManifest struct {
	DocketVersion    string   `json:"docket_version"`
	InstalledAt      string   `json:"installed_at"`
	ManagedArtifacts []string `json:"managed_artifacts"`
}

func preCommitHookPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".git", "hooks", "pre-commit")
}

func installManifestPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".docket", "install.json")
}

func claudePath(repoRoot string) string {
	return filepath.Join(repoRoot, "CLAUDE.md")
}

func preCommitHookScript() string {
	return `#!/bin/sh
set -eu

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
MSG_FILE="$REPO_ROOT/.git/COMMIT_EDITMSG"
DOCKET_BIN="${DOCKET_BIN:-docket}"

if [ ! -f "$MSG_FILE" ]; then
  echo "docket: warning: .git/COMMIT_EDITMSG not found; skipping Ticket trailer checks" >&2
  exit 0
fi

TICKETS="$(grep -Eo 'Ticket:[[:space:]]*TKT-[0-9]+' "$MSG_FILE" | sed -E 's/.*(TKT-[0-9]+)$/\1/' | sort -u || true)"
if [ -z "$TICKETS" ]; then
  echo "docket: warning: no Ticket: TKT-NNN trailer found in commit message" >&2
  exit 0
fi

for ID in $TICKETS; do
  TICKET_FILE="$REPO_ROOT/.docket/tickets/$ID.md"
  if [ -f "$TICKET_FILE" ] && grep -Eq '^state:[[:space:]]*done$' "$TICKET_FILE"; then
    echo "docket: error: referenced ticket $ID is already in done state" >&2
    exit 1
  fi
done

if [ "${DOCKET_SKIP_AC:-0}" != "1" ]; then
  for ID in $TICKETS; do
    if command -v "$DOCKET_BIN" >/dev/null 2>&1; then
      "$DOCKET_BIN" __hook-ac-check "$ID" || exit 1
    else
      echo "docket: warning: docket binary not found in PATH; skipping AC prompt checks" >&2
      break
    fi
  done
fi

if command -v "$DOCKET_BIN" >/dev/null 2>&1; then
  "$DOCKET_BIN" __hook-lock-check || true
fi

exit 0
`
}

func claudeManagedBlock() string {
	return strings.Join([]string{
		docketBlockStart,
		"## Docket Workflow",
		"",
		"- Use `docket list --state open --format context` to pick work.",
		"- Use `docket show TKT-NNN --format context` before coding.",
		"- Use `docket update TKT-NNN --state in-progress` when starting.",
		"- Use `docket ac add` / `docket ac complete` for acceptance tracking.",
		"- Add `Ticket: TKT-NNN` trailer to commit messages.",
		docketBlockEnd,
		"",
	}, "\n")
}

func ensureClaudeManagedBlock(repoRoot string) (bool, error) {
	path := claudePath(repoRoot)
	content := ""
	if data, err := os.ReadFile(path); err == nil {
		content = string(data)
	} else if !os.IsNotExist(err) {
		return false, err
	}

	block := claudeManagedBlock()
	startIdx := strings.Index(content, docketBlockStart)
	endIdx := strings.Index(content, docketBlockEnd)
	if startIdx >= 0 && endIdx > startIdx {
		endIdx += len(docketBlockEnd)
		next := strings.TrimSpace(content[:startIdx]) + "\n\n" + strings.TrimSpace(block) + "\n\n" + strings.TrimSpace(content[endIdx:])
		next = strings.TrimSpace(next) + "\n"
		if next == content {
			return false, nil
		}
		return true, os.WriteFile(path, []byte(next), 0o644)
	}

	var next string
	if strings.TrimSpace(content) == "" {
		next = block
	} else {
		next = strings.TrimSpace(content) + "\n\n" + block
	}
	next = strings.TrimSpace(next) + "\n"
	return true, os.WriteFile(path, []byte(next), 0o644)
}

func writeHook(repoRoot string) (bool, error) {
	path := preCommitHookPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	script := preCommitHookScript()
	if data, err := os.ReadFile(path); err == nil && string(data) == script {
		if chmodErr := os.Chmod(path, 0o755); chmodErr != nil {
			return false, chmodErr
		}
		return false, nil
	}
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		return false, err
	}
	return true, nil
}

func writeInstallManifest(repoRoot string) error {
	artifacts := []string{
		preCommitHookPath(repoRoot),
		claudePath(repoRoot),
	}
	sort.Strings(artifacts)
	manifest := installManifest{
		DocketVersion:    normalizeVersion(Version),
		InstalledAt:      time.Now().UTC().Format(time.RFC3339),
		ManagedArtifacts: artifacts,
	}
	path := installManifestPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func artifactStatus(repoRoot string) (hookStale bool, claudeStale bool, err error) {
	hookData, err := os.ReadFile(preCommitHookPath(repoRoot))
	if err != nil {
		hookStale = true
	} else {
		hookStale = string(hookData) != preCommitHookScript()
	}

	claudeData, err := os.ReadFile(claudePath(repoRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return hookStale, true, nil
		}
		return hookStale, true, err
	}
	content := string(claudeData)
	startIdx := strings.Index(content, docketBlockStart)
	endIdx := strings.Index(content, docketBlockEnd)
	claudeStale = startIdx < 0 || endIdx <= startIdx
	return hookStale, claudeStale, nil
}

func ensureConfigYAML(repoRoot string) error {
	path := filepath.Join(repoRoot, ".docket", "config.yaml")
	template := []string{
		"# docket managed defaults",
		"# existing keys are preserved",
		"ticket_quality_min_ac: 2",
		"ticket_quality_min_description_words: 20",
		"",
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, []byte(strings.Join(template, "\n")), 0o644)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	additions := []string{}
	if !strings.Contains(content, "ticket_quality_min_ac:") {
		additions = append(additions, "ticket_quality_min_ac: 2  # minimum recommended acceptance criteria")
	}
	if !strings.Contains(content, "ticket_quality_min_description_words:") {
		additions = append(additions, "ticket_quality_min_description_words: 20  # minimum description words")
	}
	if len(additions) == 0 {
		return nil
	}
	next := strings.TrimRight(content, "\n") + "\n" + strings.Join(additions, "\n") + "\n"
	return os.WriteFile(path, []byte(next), 0o644)
}

type migration struct {
	Version string
	Apply   func(repoRoot string) error
}

var migrations = []migration{
	{
		Version: "v0.1.0",
		Apply: func(repoRoot string) error {
			return nil
		},
	},
}

func runMigrations(repoRoot string, fromVersion string) error {
	sort.Slice(migrations, func(i, j int) bool {
		return isVersionNewer(migrations[j].Version, migrations[i].Version)
	})
	for _, m := range migrations {
		if isVersionNewer(m.Version, fromVersion) {
			if err := m.Apply(repoRoot); err != nil {
				return fmt.Errorf("migration %s failed: %w", m.Version, err)
			}
		}
	}
	return nil
}

func loadInstallManifest(repoRoot string) (installManifest, error) {
	var out installManifest
	data, err := os.ReadFile(installManifestPath(repoRoot))
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}
