package learning

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/artifacts"
)

const (
	StoreVersion = 1
)

var learnLinePattern = regexp.MustCompile(`(?i)^\s*(?:[-*]\s*)?LEARN(?:\s*\[([^\]]+)\]|\s*\(([^)]+)\)|\s+([A-Za-z][A-Za-z0-9_.-]*))?\s*:\s*(.+?)\s*$`)

type ParsedRule struct {
	Category string `json:"category"`
	Rule     string `json:"rule"`
}

type Entry struct {
	Category   string `json:"category"`
	Rule       string `json:"rule"`
	Source     string `json:"source"`
	CapturedAt string `json:"captured_at"`
}

type Snapshot struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

type IngestResult struct {
	Added   int     `json:"added"`
	Total   int     `json:"total"`
	Entries []Entry `json:"entries"`
}

type Store struct {
	repoRoot string
	now      func() time.Time
}

func NewStore(repoRoot string, now func() time.Time) *Store {
	if now == nil {
		now = func() time.Time { return time.Now().UTC().Truncate(time.Second) }
	}
	return &Store{repoRoot: repoRoot, now: now}
}

func Parse(text string) []ParsedRule {
	out := []ParsedRule{}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		m := learnLinePattern.FindStringSubmatch(line)
		if len(m) == 0 {
			continue
		}
		if strings.Contains(line, "[") && strings.Contains(line, "]") && strings.TrimSpace(m[1]) == "" {
			continue
		}
		if strings.Contains(line, "(") && strings.Contains(line, ")") && strings.TrimSpace(m[2]) == "" {
			continue
		}
		category := firstNonEmpty(m[1], m[2], m[3])
		category = normalizeCategory(category)
		rule := strings.TrimSpace(m[4])
		rule = strings.Trim(rule, "\"")
		if rule == "" {
			continue
		}
		out = append(out, ParsedRule{Category: category, Rule: rule})
	}
	return out
}

func (s *Store) Load() (Snapshot, error) {
	path := s.path()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Snapshot{Version: StoreVersion, Entries: nil}, nil
		}
		return Snapshot{}, err
	}
	var snap Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return Snapshot{}, err
	}
	if snap.Version == 0 {
		snap.Version = StoreVersion
	}
	return snap, nil
}

func (s *Store) IngestText(source, text string) (IngestResult, error) {
	parsed := Parse(text)
	snap, err := s.Load()
	if err != nil {
		return IngestResult{}, err
	}
	if snap.Version == 0 {
		snap.Version = StoreVersion
	}
	seen := map[string]struct{}{}
	for _, entry := range snap.Entries {
		seen[dedupeKey(entry.Category, entry.Rule)] = struct{}{}
	}

	added := 0
	for _, rule := range parsed {
		key := dedupeKey(rule.Category, rule.Rule)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		added++
		snap.Entries = append(snap.Entries, Entry{
			Category:   rule.Category,
			Rule:       rule.Rule,
			Source:     strings.TrimSpace(source),
			CapturedAt: s.now().Format(time.RFC3339),
		})
	}
	sort.SliceStable(snap.Entries, func(i, j int) bool {
		if snap.Entries[i].CapturedAt == snap.Entries[j].CapturedAt {
			if snap.Entries[i].Category == snap.Entries[j].Category {
				return snap.Entries[i].Rule < snap.Entries[j].Rule
			}
			return snap.Entries[i].Category < snap.Entries[j].Category
		}
		return snap.Entries[i].CapturedAt < snap.Entries[j].CapturedAt
	})
	if err := s.save(snap); err != nil {
		return IngestResult{}, err
	}
	return IngestResult{Added: added, Total: len(snap.Entries), Entries: snap.Entries}, nil
}

func (s *Store) IngestFile(source, path string) (IngestResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return IngestResult{}, err
	}
	return s.IngestText(source, string(raw))
}

func (s *Store) path() string {
	return artifacts.RepoPath(s.repoRoot, artifacts.RepoLearnRules)
}

func (s *Store) save(snapshot Snapshot) error {
	if strings.TrimSpace(s.repoRoot) == "" {
		return fmt.Errorf("repo root is required")
	}
	path := s.path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func dedupeKey(category, rule string) string {
	return strings.ToLower(strings.TrimSpace(category)) + "::" + strings.ToLower(strings.TrimSpace(rule))
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return item
		}
	}
	return ""
}

func normalizeCategory(category string) string {
	cat := strings.ToLower(strings.TrimSpace(category))
	if cat == "" {
		return "general"
	}
	return cat
}
