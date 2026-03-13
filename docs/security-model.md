# Security & Trust Model

This document describes the assets, threats, and human/agent workflows that Docket treats as the security boundary today. It is intentionally concrete so an LLM or engineer can read it without reverse-engineering the code base.

## Purpose and scope
1. Provide a trustworthy audit trail for tickets (`.docket/tickets/*.md`) even when agents or humans forget the MCP workflow.
2. Keep privileged state (locks, manifests, future keystores) outside each checked-in repository via `DOCKET_HOME`.
3. Surface health signals (tampering, incomplete acceptance criteria, file claims) before a compromised change lands in Git history.
4. Document what is *not* handled yet (e.g., OS keychains or SaaS custody) so the roadmap stays explicit.

## Trust assumptions and threat model
- **Actors**
  - *LLM agents* execute commands on the repository; they are granted the same shell access as humans but must be corralled through `docket` commands.
  - *Human maintainers* may run diagnostics, update tickets, or approve learnings; humans have the privilege to recover from bad agent output.
  - *Audit tooling* observes Git history, the manifest, and `DOCKET_HOME` to validate chronological mutations.
- **Threats**
  - Direct edits to `.docket/tickets/*.md` bypass `write_hash` signing and may corrupt the lineage of ticket metadata.
  - Uncontrolled worktrees could clash on files unless claims/locks are enforced.
  - Missing secure storage or manifest understanding makes it impossible to determine if a ticket edit was sanctioned.
  - Recovery must be possible even if the hash mismatch happened due to a bad agent or a human fiat change.

## DOCKET_HOME contract
1. **Environment gating:** Every CLI invocation calls `ensureDocketHome()` (see `cmd/root.go` and `cmd/home.go`) before doing any work. If `DOCKET_HOME` is unset, not writable, or cannot be resolved to an absolute path, the command exits with a message that includes an example such as `DOCKET_HOME=/tmp/docket-home`.
2. **Intent:** `DOCKET_HOME` is the canonical home for the *secure state machine* external to the repository. Today the variable is enforced (CLI refuses to start without it) even though most cached artifacts still live inside `.docket/`. This precondition enables future work (key enrollment, keystores, manifest storage) to trust that there is a dedicated storage area per operator.
3. **Hand-off:** `docket install` echoes the resolved path back to the operator, reminding them how to customize it. Failure on install (missing repo, unwritable `.docket`) halts before any git hooks are written.

## Tamper detection and healing
- **`write_hash` signing:** Every writer path in `internal/store/local` calls `signTicket()` before emitting `.md`. The hash is a SHA-256 over the rendered content once the `write_hash` field is blanked out (see `internal/store/local/writer.go`). This makes tampering easy to detect because the same rendering is used for both writing and validation.
- **Validation pipeline:** `docket validate` (and the lefthook pre-commit) uses `internal/store/local/ValidateFile`, which re-computes the hash via `validateSignature()` and fails with `🚨 Direct Mutation Detected. Run \`docket fix TKT-NNN\` to repair and see the correct usage.` when the regeneration does not match. That message is surfaced so humans know to run `docket fix` instead of editing `.md` manually.
- **Manifest watchdog:** The manifest (`.docket/manifest.json`) keeps a lightweight snapshot of titles, states, priorities, and parents. `DetectTamperingAll()` compares the manifest to the current files and produces `TamperChange` records. `ReconcileTampering()` applies a simple policy (accept valid schema-safe edits or reject invalid ones) and upgrades warnings into doctor findings.
- **Doctor mode:** `docket check --doctor` aggregates all validation errors, including `TamperChange` findings, turning them into `ck.Finding`s with the `V001` rule. Severity is `Error` for signatures/format issues and `Warn` otherwise. This gives operators a single report summarizing every fix needed to retain trust.
- **Fix flow:** When tampering is detected, `docket fix TKT-NNN` loads the latest ticket and optionally the last known good manifest entry, re-signs, writes the file via the store, and prints the tutor message that describes which commands should have been used (create/update/AC/comment). `docket fix --all` batches this for every invalid ticket.

## Workflow governance and lock management
- **States and transitions:** Configuration in `.docket/config.json` defines the workflow states (`backlog`, `todo`, `in-progress`, `in-review`, `done`, `archived`, etc.) and their `next` links. Agents should follow that path (e.g., `todo → in-progress → in-review → done`). `docket update` enforces this graph during CLI use.
- **Worktree scoping:** `docket worktree start TKT-NNN <path>` spawns a git worktree and records the file claims inside `locks.json` using `upsertLock`. `worktree stop` (and `lock release`) removes the claim so another actor can resume. If relations declare blocking dependencies, `worktree start` warns (or fails without `--force`) before mutating.
- **Visibility:** `docket lock status` prints which files are claimed and by which ticket/worktree. `docket status --parallel` additionally compares active locks against `relation` metadata and reports safe/risky pairs, giving operators insight into collision risks.
- **Checks:** `docket ac check` hooks into lefthook to ensure acceptance criteria are all marked (`[x]`). `docket check` (and `--doctor`) summarizes system health, while `docket validate` enforces schema hygiene. Security-critical text such as "Do not edit `.docket/tickets/*.md` directly" lives in both `lefthook.yml` and the manifest warning so agents see it in multiple places.

## Lock activation, rollback, and recovery workflows
1. **Lock activation:** Starting a worktree checks `relations.Relations` for blockers, creates a branch `docket/TKT-XXX-<timestamp>`, adds the worktree, and registers the lock with the runtime file claim tracker (`fileLock` struct). The lock file is gitignored to keep claims out of commits.
2. **Rollback detection:** If a ticket file fails validation during `docket validate` or `docket check`, the CLI refuses to progress and reports the failure path. The manifest snapshot allows `DetectTampering` to describe exactly which field drifted from the trusted values.
3. **Recovery:** `docket fix` re-signs and re-stages the ticket; `docket check --doctor` surfaces `Fix` instructions; `docket lock release`/`worktree stop` frees the lock so a different run can pick up the work.
4. **Session-based handoff:** `docket session compress` packages a transcript of a run, and `session resume` rehydrates it if another actor needs to continue. These session commands are part of the recovery story because they capture what an agent did before a rollback or tampering incident.

## Current behavior versus future hardening
- **Today's contract**
  - DOCKET_HOME is required but largely serves as a placeholder that future secure layers will build on.
  - Manifest snapshots + `write_hash` detect direct edits, and `docket fix` guides humans back to the MCP workflow.
  - Worktree locks and status checks prevent accidental file overlap without tying into an OS-level credential store.
- **Deferred or optional enhancements**
  - OS keychains or platform biometrics to encrypt private keys before writing secure metadata into `DOCKET_HOME`.
  - SaaS-backed custody (e.g., the MCP server proving a signature) instead of only local hashes.
  - Fine-grained approval gates (e.g., human review signatures before a ticket enters `done`).
  - Persistent telemetry that records each `docket fix` or `worktree start` for audit timelines.

In short, the current stack enforces the *contract* (LLMs must use `docket`, humans may restore via `docket fix`, and DOCKET_HOME is required) while leaving richer key management and telemetry to future deliverables.
