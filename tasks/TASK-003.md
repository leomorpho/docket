# TASK-003: Sequential ID generation

## Status
`[x]` done

## Depends on
TASK-002

## What this is
Implement sequential ticket ID generation (`TKT-001`, `TKT-002`, ...) backed by a counter
in `.docket/config.json`. Also defines the `Config` struct used everywhere.

## Config file: `.docket/config.json`
```json
{
  "counter": 3,
  "backend": "local",
  "states": ["backlog", "todo", "in-progress", "in-review", "done", "archived"],
  "labels": ["bug", "feature", "refactor", "chore", "llm-only", "human-only"],
  "commit_sessions": false,
  "backends": {}
}
```

`counter` = sequence number of the last created ticket. Next = counter + 1.
`backend` = which store backend to use (default `"local"`, future: `"jira"`, `"linear"`).

## Files to create

### `internal/ticket/config.go`

```go
package ticket

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
)

type BackendConfig map[string]interface{}

type Config struct {
    Counter        int                        `json:"counter"`
    Backend        string                     `json:"backend"`
    States         []string                   `json:"states"`
    Labels         []string                   `json:"labels"`
    CommitSessions bool                       `json:"commit_sessions"`
    Backends       map[string]BackendConfig   `json:"backends,omitempty"`
}

func DefaultConfig() *Config {
    return &Config{
        Counter: 0,
        Backend: "local",
        States:  []string{"backlog", "todo", "in-progress", "in-review", "done", "archived"},
        Labels:  []string{"bug", "feature", "refactor", "chore", "llm-only", "human-only"},
        CommitSessions: false,
        Backends: map[string]BackendConfig{},
    }
}

func ConfigPath(repoRoot string) string {
    return filepath.Join(repoRoot, ".docket", "config.json")
}

func LoadConfig(repoRoot string) (*Config, error) {
    data, err := os.ReadFile(ConfigPath(repoRoot))
    if err != nil {
        return nil, fmt.Errorf("docket not initialized in %s — run `docket init`", repoRoot)
    }
    var cfg Config
    if err := json.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("corrupt config.json: %w", err)
    }
    return &cfg, nil
}

func SaveConfig(repoRoot string, cfg *Config) error {
    data, err := json.MarshalIndent(cfg, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(ConfigPath(repoRoot), append(data, '\n'), 0644)
}
```

### `internal/ticket/id.go`

```go
package ticket

import "fmt"

// FormatID returns "TKT-NNN" for a given sequence number.
// Pads to 3 digits; beyond 999 produces TKT-1000 etc. — acceptable.
func FormatID(seq int) string {
    return fmt.Sprintf("TKT-%03d", seq)
}

// NextID increments the counter in config.json and returns the new ticket ID + seq.
// Not atomic across concurrent processes — acceptable for solo/small-team use.
func NextID(repoRoot string) (id string, seq int, err error) {
    cfg, err := LoadConfig(repoRoot)
    if err != nil {
        return "", 0, err
    }
    cfg.Counter++
    if err := SaveConfig(repoRoot, cfg); err != nil {
        return "", 0, fmt.Errorf("failed to increment counter: %w", err)
    }
    return FormatID(cfg.Counter), cfg.Counter, nil
}
```

## Acceptance criteria
- [x] `FormatID(1)` returns `"TKT-001"`
- [x] `FormatID(42)` returns `"TKT-042"`
- [x] `FormatID(1000)` returns `"TKT-1000"` (no panic, no truncation)
- [x] `NextID` on missing config.json returns a clear "run `docket init`" error
- [x] `NextID` called twice increments counter from 1 to 2
- [x] `LoadConfig`/`SaveConfig` round-trip preserves all fields including `backends`
- [x] `go test ./internal/ticket/...` passes using `t.TempDir()`

## Notes for LLM
- Tests must use `t.TempDir()` — never write to the real `.docket/`
- No file locking — keep it simple, document the limitation in a code comment
