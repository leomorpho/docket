# North Star

This document defines the intended product direction for Docket.

The point is to remove ambiguity. The repo has drifted between several identities:

- ticket tracker
- security/governance system
- orchestration runtime
- agent UI surface

That ambiguity is expensive. It causes product discussion to restart from first principles on every change.

This document is the answer to "what are we actually building?"

## One-Sentence Definition

Docket should be a CLI-first ticketing and orchestration runtime that turns groomed leaf tickets into validated code changes, unattended when possible, with durable artifacts that make the result easy to inspect and resume.

## Product Promise

The product promise is simple:

- give teams a backlog that agents can execute, not just read
- make autorun trustworthy by enforcing grooming and validation discipline
- let a human check back later and understand what happened quickly

If Docket cannot do those three things, it is missing the point.

## The Problem Docket Should Solve

Most "agent orchestration" products optimize the wrong thing.

They optimize:

- more agents
- more UI
- more concurrency
- more workflow ceremony

But the real failure mode in agentic development is not "not enough orchestration." It is:

- vague tickets
- weak success conditions
- poor validation
- bad handoff artifacts
- opaque runtime behavior

Docket should exist to solve that layer.

It should make agentic development feel like operating a disciplined runtime over a groomed backlog, not like supervising a chaotic swarm.

## What Docket Is

Docket is:

- a ticket system with execution semantics
- a runtime that owns state transitions and validation
- a durable record of what happened during unattended development
- a CLI-first control plane for groomed agent work

Docket is not:

- a dashboard-first orchestration product
- a generic multi-agent research sandbox
- a transcript archive disguised as memory
- a security/governance platform in the current phase

## The User Experience We Are Optimizing For

The target experience is this:

1. A human or orchestrator grooms a ticket until it is genuinely runnable.
2. Docket starts work in a fresh worktree and fresh session.
3. The agent implements the change.
4. Docket validates the result against explicit criteria.
5. Docket stores a compact run brief, updates the ticket, and records a useful commit summary.
6. Hours later, a human returns and can immediately answer:

- what completed
- what is still running
- what got stuck
- what changed
- what decisions were made
- what should happen next

That is the product.

Not a Kanban board.
Not a security approval flow.
Not a colorful monitor for multiple LLMs.

## Core Principles

### 1. Grooming Quality Drives Runtime Quality

The main determinant of autorun quality is ticket quality, not model cleverness.

If tickets are vague, underspecified, or untestable, the runtime will generate slop faster. Docket should therefore make runnable-ticket quality a first-class concept instead of treating ticket authoring as a side concern.

### 2. Only Leaf Tickets Are Executable

Parent tickets exist to organize work, not to be worked directly.

That means:

- only leaf tickets are workable
- only leaf tickets can block execution
- parent tickets summarize and group work
- parent tickets do not participate in runtime scheduling

This is a fundamental model choice, not an implementation detail.

`parent` is structure.
`blocked_by` is execution.
Those concepts should not collapse into each other.

### 3. Docket Owns State Transitions

The agent writes code.

Docket decides:

- whether a ticket was runnable
- whether a transition is allowed
- whether validation passed
- whether the run should be considered successful

The model can propose. It cannot be the authority on completion.

### 4. Validation Matters More Than Review Ceremony

Human review may happen, but it is not the default success condition.

The primary runtime target is validated output, not "ready for review." If review is usually optional, it should not define the center of the workflow.

Review should be:

- optional metadata
- an event
- a follow-up action

It should not be the required resting place for successful autorun.

### 5. Uncertainty Collapses to Serial

Parallelism is not a badge of sophistication.

If Docket is unsure, it should run serially. A trustworthy serial runtime is vastly more valuable than a flashy parallel runtime that creates merge conflicts and half-correct output.

### 6. Compact Artifacts Beat Raw Transcript Memory

Useful memory is:

- a concise run brief
- a commit summary
- updated ticket handoff
- structured validation result
- explicit stuck reason

Raw transcripts are noisy, expensive, and usually the wrong retrieval substrate. Docket should prefer compact, curated artifacts over replaying conversation logs as truth.

### 7. UI Is Secondary To Runtime Trust

A mediocre UI on top of a strong runtime is acceptable.

A polished UI on top of a weak runtime is a distraction.

The runtime must be the product center of gravity.

### 8. Trust Comes From Bounded Execution

Docket should not try to make agents "smart enough" to compensate for weak boundaries.

Trust comes from:

- strong ticket contracts
- clear runtime ownership
- bounded prompts
- deterministic validation
- inspectable artifacts

## Product Operating Model

### Workflow Model

The intended default workflow is:

`draft -> ready -> running -> validated -> archived`

Computed overlays, not primary states:

- `blocked`
- `stalled`
- `needs-input`

This keeps the system centered on execution readiness and validated output instead of human ceremony.

Directionally, this implies:

- `ready-for-review` should not be a required primary state
- `done` should mean validated completion, not "waiting on a human"
- human review should be orthogonal to the main state machine

### Ticket Model

Docket should distinguish three concepts cleanly:

1. Structural tickets
2. Execution tickets
3. Runtime overlays

Structural tickets:

- epics
- programs
- parent tickets

Execution tickets:

- leaf tickets that are small enough to run in bounded fashion

Runtime overlays:

- blocked
- stalled
- needs-input

The system should not confuse structure with execution.

### Ready Contract

A ticket should not enter `ready` until it satisfies a hard minimum contract.

That contract should include:

- clear desired outcome
- explicit acceptance criteria
- verification commands or explicit verification steps
- enough implementation context to support a bounded prompt
- declared scope or touched surfaces when relevant
- dependency edges when relevant
- a clear statement of what is out of scope

This is where Docket should be opinionated. Weak intake is the upstream cause of most runtime failure.

### Autorun Contract

Each managed run should have:

- one ticket
- one fresh worktree
- one fresh session
- one machine-readable outcome
- one validation decision owned by Docket
- one compact run brief

Each successful run should leave behind:

- the code change
- a commit summary
- updated ticket handoff
- validation evidence
- a clear next state

Each failed or stuck run should leave behind:

- the failure mode
- the stop reason
- the current state of work
- what should be resumed next

### Validation Contract

A run should not count as successful unless Docket can verify the result against explicit conditions.

At minimum, successful execution should imply:

- the intended transition is valid
- the worktree is in a good mergeable state
- a commit exists when expected
- validation commands or steps have passed
- the ticket and run artifacts have been updated

"The agent said it is done" is not a validation strategy.

## Scheduler And Parallelism Stance

V1 should be single-lane and serial.

The scheduler abstraction can exist, but the product should not spend its core energy on broad parallel execution yet.

If lane support is added later, the rule should remain conservative:

- serial within a lane
- parallel across lanes
- uncertainty collapses to the same lane

Lane assignment should eventually respect:

- explicit dependencies
- leaf-ticket boundaries
- touched surfaces
- test overlap
- historical conflict evidence

But that is a later phase. The immediate product is a reliable serial runtime.

## Retrieval And Memory Stance

Retrieval should help execution quality, not act as the runtime authority.

Good retrieval inputs:

- current ticket
- dependency tickets
- adjacent feature tickets
- repo docs
- code summaries
- prior run briefs
- commit summaries

Bad retrieval defaults:

- raw full transcripts
- unfiltered chat history
- vague semantic similarity with no execution meaning

If retrieval is added, it should serve:

- better prompts
- better diagnostics
- better resumption

It should not replace deterministic workflow and validation rules.

## Human Review Stance

Human review is by exception.

That means:

- successful autorun should end in `validated`
- review can happen after validation if desired
- review should be optional, not structurally mandatory

This is important because mandatory review states create artificial blocking in a system whose goal is unattended execution.

## Security And Governance Stance

Security/governance is not a current product pillar.

That does not mean it is permanently unimportant. It means it should not define the product while Docket is still proving the core runtime.

For now, product direction should not be driven by:

- secure mode
- privileged transition systems
- approval locks
- governance-heavy workflow semantics

Those concerns can be revisited later, ideally as isolated subsystems rather than as the repo's conceptual center.

## Explicit Non-Goals For Now

These are not near-term product goals:

- broad multi-agent swarms
- optimistic default parallelism
- transcript-heavy memory systems
- merge-conflict learning systems
- workflow approval bureaucracy
- security/governance as the main story
- dashboard-first orchestration
- replacing grooming discipline with smarter prompting

## Decision Rubric

When evaluating a feature, ask:

1. Does this make groomed tickets easier to execute reliably?
2. Does this improve validation, resumability, or artifact quality?
3. Does this reduce ambiguity in the runtime model?
4. Does this help unattended serial execution?
5. Would this still matter if the UI did not exist?

If the answer to most of those is no, it is probably not core.

More specifically:

- if a feature adds more ceremony than execution quality, it is probably off track
- if a feature makes state transitions less clear, it is off track
- if a feature assumes stronger agent judgment instead of stronger Docket control, it is off track
- if a feature makes debugging or resumption harder, it is off track

## Success Criteria

Docket is on track when:

- groomed leaf tickets can be run unattended in serial
- successful runs land in `validated` with strong artifacts
- parent tickets organize work without blocking execution
- stuck runs are explicit, inspectable, and resumable
- repo history tells the story of execution without transcript spelunking
- a returning human can understand the state of the system in minutes

## Repository Implications

Directionally, the repo should optimize for:

- ticket quality
- orchestration
- validation
- resumability
- compact artifacts
- leaf-ticket execution discipline

Legacy security/governance code may remain in the tree for a while, but it should not define the strategic direction.

## Near-Term Doctrine

For the next phase, the practical doctrine should be:

- simplify the workflow
- enforce runnable-ticket quality
- make `validated` the success target
- strengthen serial autorun
- preserve compact run artifacts
- keep parent tickets structural only
- defer sophisticated parallelism and memory systems

That is the shortest path to a product that is actually good.
