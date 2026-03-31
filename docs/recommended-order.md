# Recommended Order

This document turns the current product direction into an execution order for the repository.

The sequence is intentionally conservative. The aim is to get to a trustworthy serial autorun loop before adding broader memory or parallelism features.

## 1. Freeze Security/Governance Expansion

Stop investing in new secure-mode, workflow-lock, approval, or privileged-transition work for now.

Why:

- it is not the current product goal
- it adds conceptual weight to every runtime change
- it obscures the simpler ticketing + orchestration identity

Outcome:

- no new feature work in that area
- existing behavior treated as legacy until deliberately removed or isolated later

## 2. Align the Product Model

Move the conceptual workflow to:

`draft -> ready -> running -> validated -> archived`

Why:

- `ready` expresses grooming quality
- `running` expresses active execution
- `validated` expresses the real runtime success target
- this removes review ceremony from the core loop

Outcome:

- a smaller, execution-oriented state model

## 3. Make `ready` a Hard Gate

Define and enforce the minimum ticket quality bar for runnable work.

A ticket should not be runnable until it has:

- clear outcome
- acceptance criteria
- verification commands or explicit verification steps
- declared scope/touched surfaces
- dependencies when relevant

Why:

- the runtime can only be as good as the ticket intake

Outcome:

- fewer vague tickets entering autorun
- better prompts
- less downstream slop

## 4. Change Successful Runs to End in `validated`

Successful autorun should not funnel into `in-review` as the default destination.

Why:

- human review is optional, not the primary runtime success condition
- the system should optimize for validated output, not waiting states

Outcome:

- successful machine-owned runs land in a terminal success state that matches product intent

## 5. Persist Compact Run Artifacts

Every managed run should produce:

- local run brief
- updated ticket handoff
- commit-time summary
- validation result
- clear stuck reason when applicable

Why:

- this is the minimum useful "memory" for unattended execution

Outcome:

- a human can return later and understand what happened quickly

## 6. Make Stuck Runs First-Class

Treat stuck runs as a normal runtime outcome with explicit resume support.

Why:

- unattended execution is only valuable if failure is inspectable and resumable

Outcome:

- `run-status` and `run-resume` become core runtime surfaces instead of side diagnostics

## 7. Keep Scheduling Single-Lane

Retain the scheduler abstraction, but run only one lane by default.

Why:

- serial execution is the shortest path to a trustworthy product
- premature parallelism creates merge and quality problems before the base loop is stable

Outcome:

- Docket has a clean place to add lanes later without taking on the complexity now

## 8. Measure Failure Before Adding Parallelism

Once the serial loop is stable, measure:

- stuck-rate
- validation failure rate
- rerun frequency
- handoff quality
- resume success rate

Why:

- those numbers should drive future investments

Outcome:

- later decisions about lanes or memory are grounded in real operational failure modes

## 9. Revisit Minimal Lane Support Later

Only after serial autorun is strong should Docket try:

- more than one lane
- conservative same-lane defaults on uncertainty
- basic overlap heuristics

Why:

- parallelism should be earned by runtime maturity

Outcome:

- future parallel support is layered on top of a working system instead of compensating for a weak one

## 10. Leave Rich Memory and Conflict Learning for a Later Phase

Do not make transcript retrieval, learning-driven scheduling, or merge-resolver agents part of the near-term critical path.

Why:

- those systems are useful only after the serial runtime produces stable, trustworthy artifacts

Outcome:

- the roadmap stays focused on the shortest path to a good product

## Immediate Repo-Level Deliverables

The first concrete repo deliverables implied by this order are:

1. North-star documentation
2. Workflow simplification plan
3. `ready` gate definition
4. `validated`-state finalization path
5. Compact run brief and commit-summary artifacts

Everything else is downstream of those five pieces.
