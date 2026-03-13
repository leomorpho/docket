# docket

A git-native ticket system built for human + LLM agentic workflows.

Not JIRA. Not a markdown checklist. A structured, append-only, auditable issue tracker
that lives in your repo and works as a first-class tool for AI agents.

## What it does

- Tracks tickets as markdown files with YAML frontmatter at `.docket/tickets/TKT-XXX.md`
- Gives LLMs **forward context**: ticket descriptions, acceptance criteria, handoff summaries, plans
- Gives LLMs **backward context**: `docket blame file:line` → commit → ticket chain
- Interactive kanban board (`docket board`) with bubbletea TUI
- Pre-commit gate: `docket ac check TKT-XXX` exits 1 if acceptance criteria incomplete
- Dual surface: same binary works as CLI and as MCP server (`docket serve --mcp`)

## How it stores data

```
.docket/
├── config.json               # sequential counter, workflow config — committed
├── tickets/
│   ├── TKT-001.md            # ticket markdown source of truth — committed
│   └── TKT-001/sessions/     # conversation transcripts — gitignored by default
└── index.db                  # SQLite query cache — always gitignored, rebuilt on demand
```

Markdown ticket files are the source of truth. SQLite is a cache only and is never committed.

## Ticket states

```
backlog → todo → in-progress → [blocked*] → in-review → done → archived
```

`blocked` is computed automatically from unresolved `blocked-by` dependencies.

## DOCKET_HOME

Docket keeps its secure metadata in a repository-isolated root defined by `DOCKET_HOME`. Set `DOCKET_HOME` to an absolute, writable directory (for example, `DOCKET_HOME=$HOME/.docket-home`) before running commands. The CLI fails immediately if the variable is unset or if the path cannot be used, forcing secure storage to be configured up front.

## Quick start

```bash
# install
go install github.com/leomorpho/docket@latest

# initialize in your repo
docket init

# create a ticket
docket create --title "Add auth middleware" --priority 1 --labels feature

# interactive kanban board
docket board

# get LLM-optimized context for a file
docket context src/auth/middleware.go

# blame a line and get ticket history
docket blame src/auth/middleware.go:42

# run as MCP server
docket serve --mcp
```

## For LLMs

Run `docket help-json` to get a machine-readable manifest of all commands and their schemas.

Always use `docket` commands to modify ticket state — never edit `.docket/` files directly.

When starting work, run `docket list --state open --format context` to find relevant tickets.

When finishing work, run `docket session compress TKT-XXX` to generate a handoff summary.

## Design decisions

See [ARCHITECTURE.md](./ARCHITECTURE.md) for the full rationale behind every design choice.

## Semantic mode

Semantic mode adds local-only ticket embeddings and hybrid related-ticket lookup. The current
provider is `uv` plus `sentence-transformers`, and all semantic artifacts are rebuildable caches.

### Commands

```bash
# inspect semantic provider and index readiness
docket semantic status

# incrementally rebuild embeddings for changed tickets
docket semantic rebuild

# force a clean rebuild of local semantic state
docket semantic rebuild --full

# find related tickets with lexical-only, fallback, or required semantic mode
docket related TKT-001 --semantic off
docket related TKT-001 --semantic auto
docket related TKT-001 --semantic on
```

### Config

Semantic config lives in `.docket/config.json` under `semantic`.

```json
{
  "semantic": {
    "enabled": false,
    "provider": "uv",
    "model": "sentence-transformers/all-MiniLM-L6-v2",
    "hf_home": "~/.cache/docket/hf",
    "sentence_transformers_home": "~/.cache/docket/sbert",
    "uv_cache_dir": "~/.cache/docket/uv",
    "lexical_weight": 0.35,
    "vector_weight": 0.65,
    "title_weight": 3.0,
    "description_weight": 1.5,
    "ac_weight": 2.0,
    "handoff_weight": 1.25
  }
}
```

Config keys:

| Key | Meaning | Default |
| --- | --- | --- |
| `semantic.enabled` | Enables semantic provider use for commands that support it. | `false` |
| `semantic.provider` | Embedding provider implementation. | `uv` |
| `semantic.model` | Sentence-transformers model name passed to the bridge script. | `sentence-transformers/all-MiniLM-L6-v2` |
| `semantic.hf_home` | Hugging Face cache root used by the provider subprocess. | `~/.cache/docket/hf` |
| `semantic.sentence_transformers_home` | Sentence-transformers cache directory used by the provider subprocess. | `~/.cache/docket/sbert` |
| `semantic.uv_cache_dir` | `uv` package cache directory used by the provider subprocess. | `~/.cache/docket/uv` |
| `semantic.lexical_weight` | Hybrid combiner weight for lexical ranking. | `0.35` |
| `semantic.vector_weight` | Hybrid combiner weight for vector ranking. | `0.65` |
| `semantic.title_weight` | Relative field weight for ticket titles in lexical and semantic ranking. | `3.0` |
| `semantic.description_weight` | Relative field weight for ticket descriptions in lexical and semantic ranking. | `1.5` |
| `semantic.ac_weight` | Relative field weight for acceptance criteria in lexical and semantic ranking. | `2.0` |
| `semantic.handoff_weight` | Relative field weight for handoff text in lexical and semantic ranking. | `1.25` |

Environment overrides:

- `DOCKET_SEMANTIC_ENABLED`
- `DOCKET_SEMANTIC_PROVIDER`
- `DOCKET_SEMANTIC_MODEL`
- `DOCKET_SEMANTIC_HF_HOME`
- `DOCKET_SEMANTIC_SENTENCE_TRANSFORMERS_HOME`
- `DOCKET_SEMANTIC_UV_CACHE_DIR`
- `DOCKET_SEMANTIC_LEXICAL_WEIGHT`
- `DOCKET_SEMANTIC_VECTOR_WEIGHT`
- `DOCKET_SEMANTIC_TITLE_WEIGHT`
- `DOCKET_SEMANTIC_DESCRIPTION_WEIGHT`
- `DOCKET_SEMANTIC_AC_WEIGHT`
- `DOCKET_SEMANTIC_HANDOFF_WEIGHT`

Precedence:

- Command flags control runtime behavior first where supported. Today that means `docket related --semantic off|auto|on` selects lexical-only, fallback, or required semantic execution for that invocation.
- Environment variables override `.docket/config.json`.
- `.docket/config.json` overrides built-in defaults.
- Built-in defaults apply when neither flags, env vars, nor config provide a value.

### Local caches and artifacts

Default local cache paths:

- Hugging Face model cache: `~/.cache/docket/hf`
- Sentence-transformers cache: `~/.cache/docket/sbert`
- `uv` package cache: `~/.cache/docket/uv`

Default local semantic artifacts under the repo:

- `.docket/semantic/metadata.json`
- `.docket/semantic/vector/`

These semantic caches and indexes are local-only, gitignored, and rebuildable. They should not be committed.

### Provider requirement

The initial semantic provider requires [`uv`](https://docs.astral.sh/uv/) to be installed locally.

If `uv` is unavailable:

- `docket semantic status` reports the provider as unavailable and includes a warning.
- `docket related --semantic auto` falls back to lexical-only results with warnings.
- `docket related --semantic on` fails instead of silently falling back.

### Rebuild workflow

First run behavior:

- The first semantic rebuild downloads Python packages through `uv` and downloads the configured sentence-transformers model into the local cache directories.
- Later runs reuse those caches and only re-embed changed ticket chunks when you use the default incremental rebuild path.

Normal rebuild flow:

- `docket semantic rebuild` performs an incremental rebuild.
- Incremental rebuild hashes ticket chunks and only adds, updates, or deletes the chunks that changed.
- `docket semantic status` shows provider readiness, cache paths, freshness, and current local index metadata.

Use a full rebuild when:

- The local vector index is corrupted.
- The semantic metadata version changed.
- The provider or model changed and you want to rebuild from scratch.

`docket semantic rebuild --full` removes and recreates the local vector store, resets metadata, and reindexes every ticket chunk.

## Development

Work is tracked in [`tasks/`](./tasks/). Each file is a self-contained atomic task with
full context so any LLM can pick it up independently.

See [`tasks/OVERVIEW.md`](./tasks/OVERVIEW.md) for the task graph and current status.
