package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/artifacts"
	"github.com/leomorpho/docket/internal/ticket"
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

func starterScaffoldLayout(repoRoot string) []string {
	return []string{
		artifacts.MustRelPath(artifacts.RepoInstallManifest),
		filepath.Join(".git", "hooks", "commit-msg"),
		filepath.Join(".git", "hooks", "pre-commit"),
		".gitignore",
		filepath.Base(claudePath(repoRoot)),
	}
}

func starterScaffoldManagedArtifacts(repoRoot string) []string {
	managed := []string{
		claudePath(repoRoot),
		commitMsgHookPath(repoRoot),
		preCommitHookPath(repoRoot),
	}
	sort.Strings(managed)
	return managed
}

func validateStarterScaffoldLayout(repoRoot string) error {
	for _, rel := range starterScaffoldLayout(repoRoot) {
		path := filepath.Join(repoRoot, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("starter scaffold missing %s", rel)
			}
			return fmt.Errorf("read starter scaffold %s: %w", rel, err)
		}
		if rel == ".gitignore" && !strings.Contains(string(data), artifacts.CanonicalLocalRootRelPath()+"/") {
			return fmt.Errorf("starter scaffold missing %s entry in %s", artifacts.CanonicalLocalRootRelPath()+"/", rel)
		}
	}
	return nil
}

func preCommitHookPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".git", "hooks", "pre-commit")
}

func commitMsgHookPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".git", "hooks", "commit-msg")
}

func installManifestPath(repoRoot string) string {
	return artifacts.RepoPath(repoRoot, artifacts.RepoInstallManifest)
}

func claudePath(repoRoot string) string {
	return filepath.Join(repoRoot, "CLAUDE.md")
}

func preCommitHookScript() string {
	return `#!/bin/sh
set -eu

DOCKET_BIN="${DOCKET_BIN:-docket}"

if command -v "$DOCKET_BIN" >/dev/null 2>&1; then
  "$DOCKET_BIN" __hook-lock-check || true
fi

exit 0
`
}

func commitMsgHookScript() string {
	ticketsRelDir := filepath.ToSlash(artifacts.MustRelPath(artifacts.RepoTicketsDir))
	configRelPath := filepath.ToSlash(artifacts.MustRelPath(artifacts.RepoConfigJSON))
	return `#!/bin/sh
set -eu

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
MSG_FILE="${1:-$REPO_ROOT/.git/COMMIT_EDITMSG}"
DOCKET_BIN="${DOCKET_BIN:-docket}"

if [ ! -f "$MSG_FILE" ]; then
  echo "docket: warning: commit message file not found; skipping Ticket trailer checks" >&2
  exit 0
fi

TICKETS="$(grep -Eo 'Ticket:[[:space:]]*TKT-[0-9]+' "$MSG_FILE" | sed -E 's/.*(TKT-[0-9]+)$/\1/' | sort -u || true)"
if [ -z "$TICKETS" ]; then
  echo "docket: warning: no Ticket: TKT-NNN trailer found in commit message" >&2
  exit 0
fi

if [ -d "$REPO_ROOT/.git" ]; then
  echo "docket: error: Ticket-linked commits must be created from a dedicated worktree, not the primary checkout" >&2
  exit 1
fi

for ID in $TICKETS; do
  TICKET_FILE="$REPO_ROOT/` + ticketsRelDir + `/$ID.md"
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$REPO_ROOT" "$ID" <<'PY' || exit 1
import json
import pathlib
import re
import sys

repo_root = pathlib.Path(sys.argv[1])
ticket_id = sys.argv[2]
ticket_path = repo_root / pathlib.PurePosixPath("` + ticketsRelDir + `") / f"{ticket_id}.md"
if not ticket_path.exists():
    raise SystemExit(0)

match = re.search(r'^state:\s*"?([^"\n]+)"?\s*$', ticket_path.read_text(), re.MULTILINE)
if not match:
    raise SystemExit(0)
state = match.group(1).strip()

closed_states = {"done", "archived"}
config_path = repo_root / pathlib.PurePosixPath("` + configRelPath + `")
try:
    config = json.loads(config_path.read_text())
    states = config.get("states") or {}
    derived = {name for name, meta in states.items() if meta.get("open") is False}
    if derived:
        closed_states = derived
except Exception:
    pass

if state in closed_states:
    print(f"docket: error: referenced ticket {ticket_id} is already in closed state {state}", file=sys.stderr)
    raise SystemExit(1)
PY
  elif [ -f "$TICKET_FILE" ] && grep -Eq '^state:[[:space:]]*done$' "$TICKET_FILE"; then
    echo "docket: error: referenced ticket $ID is already in done state" >&2
    exit 1
  fi
done

if [ "${DOCKET_SKIP_AC:-0}" != "1" ]; then
  for ID in $TICKETS; do
    if command -v "$DOCKET_BIN" >/dev/null 2>&1; then
      "$DOCKET_BIN" __hook-ac-check "$ID" || exit 1
    else
      echo "docket: warning: docket binary not found in PATH; skipping AC hook checks" >&2
      break
    fi
  done
fi

exit 0
`
}

func claudeManagedBlock(repoRoot string) string {
	cfg, err := ticket.LoadConfig(repoRoot)
	if err != nil {
		cfg = ticket.DefaultConfig()
	}
	return strings.Join([]string{
		docketBlockStart,
		"## Docket Workflow",
		"",
		"- Use `docket list --state open --format context` to pick work.",
		"- Use `docket show TKT-NNN --format context` before coding.",
		fmt.Sprintf("- Use `docket update TKT-NNN --state %s` when moving a ticket into active work.", activeWorkflowState(cfg)),
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

	block := claudeManagedBlock(repoRoot)
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
	prePath := preCommitHookPath(repoRoot)
	commitPath := commitMsgHookPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(prePath), 0o755); err != nil {
		return false, err
	}
	wroteAny := false
	hooks := []struct {
		path   string
		script string
	}{
		{path: prePath, script: preCommitHookScript()},
		{path: commitPath, script: commitMsgHookScript()},
	}
	for _, hook := range hooks {
		if data, err := os.ReadFile(hook.path); err == nil && string(data) == hook.script {
			if chmodErr := os.Chmod(hook.path, 0o755); chmodErr != nil {
				return false, chmodErr
			}
			continue
		}
		if err := os.WriteFile(hook.path, []byte(hook.script), 0o755); err != nil {
			return false, err
		}
		wroteAny = true
	}
	return wroteAny, nil
}

func writeInstallManifest(repoRoot string) error {
	manifest := installManifest{
		DocketVersion:    normalizeVersion(Version),
		InstalledAt:      time.Now().UTC().Format(time.RFC3339),
		ManagedArtifacts: starterScaffoldManagedArtifacts(repoRoot),
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
	hooks := []struct {
		path   string
		script string
	}{
		{path: preCommitHookPath(repoRoot), script: preCommitHookScript()},
		{path: commitMsgHookPath(repoRoot), script: commitMsgHookScript()},
	}
	for _, hook := range hooks {
		hookData, readErr := os.ReadFile(hook.path)
		if readErr != nil {
			hookStale = true
			break
		}
		if string(hookData) != hook.script {
			hookStale = true
			break
		}
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
	path := artifacts.RepoPath(repoRoot, artifacts.RepoConfigYAML)
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
