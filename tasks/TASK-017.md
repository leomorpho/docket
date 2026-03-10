# TASK-017: Session management

## Status
`[x]` done

## Depends on
TASK-011 (comment command — same append pattern)

## What this is
Manage LLM conversation sessions attached to tickets. Sessions are stored in
`.docket/tickets/TKT-001/sessions/` (gitignored by default). The key operation
is `compress` — summarize a session into a structured handoff summary.

## Commands

### `docket session attach <TKT-NNN> --file <path>`
Attach a conversation log file to a ticket.
```
docket session attach TKT-001 --file /tmp/claude-session-2026-03-09.jsonl
```
Copies the file to `.docket/tickets/TKT-001/sessions/2026-03-09T142200Z.jsonl`.
Appends a comment: `Session attached: sessions/2026-03-09T142200Z.jsonl`

### `docket session list <TKT-NNN>`
List attached session files for a ticket.
```
Sessions for TKT-001:
  2026-03-09T142200Z.jsonl  (2.3 MB)
  2026-03-08T091500Z.jsonl  (1.1 MB, compressed → handoff updated)
```

### `docket session compress <TKT-NNN> [--session <filename>] [--keep]`
Summarize the latest session (or specified session) into a handoff summary.

Behavior:
1. Read the session file content
2. Output a structured prompt to stdout and instruct the calling LLM to produce a handoff
   summary in the required format (see below) — OR accept `--summary-file <path>` to
   read a pre-written summary
3. Write the summary to the `## Handoff` section of the ticket markdown file
4. Append a comment: `Session compressed. Handoff updated.`
5. Unless `--keep`: rename session file to `*.compressed` (keeps it but marks as processed)

**Note on LLM integration:** `docket session compress` does not call an LLM API itself.
Instead it outputs a prompt that the calling agent uses to produce the summary, then
the agent calls `docket session compress --summary-file <path>` with the result.
This keeps the binary dependency-free from any LLM SDK.

## Required handoff summary format
The `## Handoff` section must contain these subsections (validated by `docket validate`):
```markdown
## Handoff

*Last updated: 2026-03-09T15:00:00Z by agent:claude-sonnet-4-6*

**Current state:** What was done in this session and what the ticket's current state is.

**Decisions made:** Key decisions and their rationale. What alternatives were considered.

**Files touched:** List of files created or modified, with brief description of changes.

**Remaining work:** Concrete next steps. Specific enough for a new LLM to continue.

**AC status:** Which AC items are done, which remain, any blockers.
```

## Files to create
- `cmd/session.go` (cobra parent command)
- `cmd/session_attach.go`
- `cmd/session_list.go`
- `cmd/session_compress.go`
- `internal/store/local/sessions.go` (file copy, list, compress logic)

## Acceptance criteria
- [x] `docket session attach TKT-001 --file /tmp/log.jsonl` copies file to sessions dir
- [x] `docket session list TKT-001` lists session files with sizes
- [x] `docket session compress TKT-001 --summary-file /tmp/summary.md` writes to `## Handoff`
- [x] `docket session compress` without `--summary-file` outputs the prompt to stdout
- [x] Compress without `--keep` renames session to `.compressed`
- [x] A comment is appended to the ticket on both attach and compress
- [x] Sessions dir is gitignored (from TASK-006 init)
- [x] `go test ./cmd/...` and `./internal/store/local/...` pass

## Notes for LLM
- Do NOT add any HTTP client or LLM API calls to this binary — stay dependency-free
- The compress prompt output should be clear enough that any LLM can produce a valid handoff
- Session files can be any format (JSONL, plain text) — docket treats them as opaque blobs
