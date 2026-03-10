# TASK-012: SQLite index cache

## Status
`[x]` done

## Depends on
TASK-011 (all basic CRUD commands done, store is stable)

## What this is
Add a SQLite query cache derived from parsing markdown ticket files. Speeds up `docket list`,
`docket board`, and `docket check`. Never committed to git (always in `.gitignore`).

## Why
Parsing all `.md` files on every `docket list` call is fine for < 100 tickets. Beyond that,
or for the interactive board TUI, you want fast indexed queries without re-parsing everything.
The cache is a derived artifact — rebuilt from source-of-truth markdown files on demand.

## Design
- Cache lives at `.docket/index.db`
- Rebuilt automatically when the CLI detects any `.md` file newer than `index.db`
- Can be manually rebuilt: `docket sync`
- Uses `modernc.org/sqlite` (pure Go, no CGO)

## Schema

```sql
CREATE TABLE tickets (
    id            TEXT PRIMARY KEY,
    seq           INTEGER NOT NULL,
    state         TEXT NOT NULL,
    priority      INTEGER NOT NULL DEFAULT 10,
    title         TEXT NOT NULL,
    created_by    TEXT NOT NULL,
    created_at    DATETIME NOT NULL,
    updated_at    DATETIME NOT NULL,
    is_blocked    INTEGER NOT NULL DEFAULT 0,  -- computed: len(blocked_by) > 0
    ac_total      INTEGER NOT NULL DEFAULT 0,
    ac_done       INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE labels (
    ticket_id  TEXT NOT NULL REFERENCES tickets(id),
    label      TEXT NOT NULL
);

CREATE TABLE blocked_by (
    ticket_id  TEXT NOT NULL REFERENCES tickets(id),
    blocks_id  TEXT NOT NULL
);

CREATE TABLE linked_commits (
    ticket_id  TEXT NOT NULL REFERENCES tickets(id),
    sha        TEXT NOT NULL
);

CREATE INDEX idx_tickets_state ON tickets(state);
CREATE INDEX idx_tickets_priority ON tickets(priority);
CREATE INDEX idx_labels_ticket ON labels(ticket_id);
```

## Files to create

### `internal/store/local/index.go`

```go
package local

// IndexPath returns the path to the SQLite cache file.
func (s *Store) IndexPath() string

// SyncIndex rebuilds the SQLite cache from all ticket markdown files.
// Called automatically when index is stale; can be called explicitly.
func (s *Store) SyncIndex(ctx context.Context) error

// isIndexStale returns true if any .md file in tickets/ is newer than index.db.
func (s *Store) isIndexStale() bool

// ensureIndex syncs the index if stale. Call at the start of any list/filter operation.
func (s *Store) ensureIndex(ctx context.Context) error
```

Update `ListTickets` to call `ensureIndex` and query SQLite instead of parsing all files.
`GetTicket` continues to read the markdown file directly (single ticket, fast enough).

### `cmd/sync.go`

```
docket sync
```

Explicitly rebuilds the index. Human output: `Synced index: 12 tickets.`

## Dependencies to add
```bash
go get modernc.org/sqlite
```

## Acceptance criteria
- [x] `docket sync` creates `.docket/index.db`
- [x] After sync, `docket list` queries SQLite (verify with a timing test on 50+ tickets)
- [x] If `index.db` doesn't exist, `docket list` builds it automatically first
- [x] If any `.md` file is newer than `index.db`, the index is rebuilt before querying
- [x] `index.db` is not tracked by git (confirmed by `.gitignore` from TASK-006)
- [x] `docket sync` after deleting a ticket removes it from the index
- [x] `go test ./internal/store/local/...` passes

## Notes for LLM
- `ensureIndex` should be lightweight — just stat the files, don't parse unless stale
- The index is a cache, not a source of truth — if in doubt, re-parse from markdown
- `modernc.org/sqlite` requires no CGO and produces a single static binary
