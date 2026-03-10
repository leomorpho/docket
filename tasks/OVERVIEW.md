# Task Overview

All planned work for `docket`, broken into atomic tasks. Each task file is self-contained:
a single LLM can read it and complete the work without needing the rest of the conversation.

**Before starting any task, read:**
- [`ARCHITECTURE.md`](../ARCHITECTURE.md) — design decisions and rationale
- [`README.md`](../README.md) — what docket is and how it works

## Status legend
- `[ ]` not started
- `[~]` in progress
- `[x]` done

## Task graph

```
TASK-001 (scaffolding)
  └── TASK-002 (data types)
        └── TASK-003 (ID generation)
              └── TASK-004 (markdown store: read/write/parse .md files)
                    └── TASK-005 (schema validator: docket validate)
                          └── TASK-006 (docket init)
                                ├── TASK-007 (docket create)
                                ├── TASK-008 (docket list)
                                ├── TASK-009 (docket show)
                                ├── TASK-010 (docket update)
                                └── TASK-011 (docket comment)
                                      └── TASK-012 (SQLite index cache)
                                            ├── TASK-013 (docket board TUI)
                                            └── TASK-016 (docket context)
                          └── TASK-014 (git blame integration)
                                └── TASK-015 (inline annotation scan + refs)
                                      └── TASK-016 (docket context)
                          └── TASK-017 (session management)
                                └── TASK-018 (acceptance criteria)
                                      └── TASK-019 (docket check staleness)
                                            └── TASK-020 (docket help-json)
                                                  └── TASK-021 (MCP server)
                                                        └── TASK-022 (lefthook + CLAUDE.md template)
                                                              └── TASK-023 (bootstrap docket itself)
```

## Task list

| Task | Title | Depends on | Status |
|------|-------|------------|--------|
| [TASK-001](./TASK-001.md) | Project scaffolding | — | [x] |
| [TASK-002](./TASK-002.md) | Core data types | TASK-001 | [x] |
| [TASK-003](./TASK-003.md) | Sequential ID generation | TASK-002 | [x] |
| [TASK-004](./TASK-004.md) | Markdown store (read/write/parse) | TASK-003 | [x] |
| [TASK-005](./TASK-005.md) | Schema validator (`docket validate`) | TASK-004 | [x] |
| [TASK-006](./TASK-006.md) | `docket init` command | TASK-005 | [x] |
| [TASK-007](./TASK-007.md) | `docket create` command | TASK-006 | [x] |
| [TASK-008](./TASK-008.md) | `docket list` command | TASK-006 | [x] |
| [TASK-009](./TASK-009.md) | `docket show` command | TASK-006 | [x] |
| [TASK-010](./TASK-010.md) | `docket update` command | TASK-006 | [x] |
| [TASK-011](./TASK-011.md) | `docket comment` command | TASK-006 | [x] |
| [TASK-012](./TASK-012.md) | SQLite index cache | TASK-011 | [x] |
| [TASK-013](./TASK-013.md) | `docket board` bubbletea TUI | TASK-012 | [x] |
| [TASK-014](./TASK-014.md) | Git blame integration | TASK-005 | [x] |
| [TASK-015](./TASK-015.md) | Inline annotation scan + refs | TASK-014 | [x] |
| [TASK-016](./TASK-016.md) | `docket context` command | TASK-012, TASK-015 | [ ] |
| [TASK-017](./TASK-017.md) | Session management | TASK-011 | [ ] |
| [TASK-018](./TASK-018.md) | Acceptance criteria | TASK-017 | [ ] |
| [TASK-019](./TASK-019.md) | `docket check` staleness | TASK-018 | [ ] |
| [TASK-020](./TASK-020.md) | `docket help-json` manifest | TASK-019 | [ ] |
| [TASK-021](./TASK-021.md) | MCP server | TASK-020 | [ ] |
| [TASK-022](./TASK-022.md) | lefthook + CLAUDE.md template | TASK-021 | [ ] |
| [TASK-023](./TASK-023.md) | Bootstrap docket to track itself | TASK-022 | [ ] |
