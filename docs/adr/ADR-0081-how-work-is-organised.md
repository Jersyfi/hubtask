# ADR-0081 — How work on Hubtask is organised

**Status:** accepted · **Date:** 2026-10-07 · **Decided:** 2026-10-07, by the owner

**Rule lives in:** [AGENTS.md](../../AGENTS.md), [docs/backlog/README.md](../backlog/README.md),
[known-traps.md](../architecture/known-traps.md)

## Context

Work stalled. Tasks raised questions during or after the build that preparation should have
settled: of 47 questions in one session, 31 were answerable from documents that existed before the
code. A milestone grew from 15 to 37 tasks while 11 of its first 15 stayed unbuilt. The causes were
measured: a task's premise was never checked against the code; nobody walked doors × states ×
kinds of account; standing rules were not applied as checks; the only systematic check ran after
the build; questions had no fixed place or time; and a milestone stayed open to every new idea.

At the same time the knowledge to avoid this was spread out: rules in nine `CLAUDE.md` files, in
engineering guidelines nobody was pointed to, in about 80 ADRs whose current rule had to be
assembled from chains of amendments, and in 147 private notes of one coding agent on one machine.
Only Claude Code could read the instructions, and nothing worked from another machine.

The owner works only through coding sessions and decides direction; the community and close
collaborators work with tools of their own; a tool that steers coding agents will follow.

## Decision

1. **One place per kind of knowledge** (the table in `AGENTS.md`). Subject documents hold every
   current rule; an ADR records why and when, is not edited after it is accepted, and names the
   section that holds its rule in a `Rule lives in` line.
2. **`AGENTS.md` is the one instruction** for every coding worker, at the root and in eight
   directories. No `CLAUDE.md` remains: one beside an `AGENTS.md` would hide it from Claude Code,
   which otherwise reads `AGENTS.md` exactly as it read `CLAUDE.md`.
3. **A task is settled before its code**: a short readiness record — premise, coverage and the
   door × state matrix, decisions with their decider, proof — attacked by a reviewer who did not
   write it, committed first.
4. **A milestone fixes what it delivers** (`Delivers:`, use case checks) when the owner releases
   it; its tasks may move freely within that, and it is done when every delivered check holds.
5. **Questions and findings are labelled issues**: `decision` for the owner, collected and put to
   the owner once, answered into the document it governs; `finding` for anything outside a task,
   taken in or closed at the next cut.
6. **Every rule says what checks it** — a gate, partly a gate, the owner, or nothing and why.
7. **Knowledge stays in the repository**, never only in a tool's private notes.

## Consequences

* A worker reads about 16,000 words for a task instead of about 37,000, and every word is current.
* The process can be followed by any person or tool, from any machine; the state a steering tool
  needs is in files, labels and pull requests.
* Some rules are honestly marked as not yet checked; their gates follow.
* The readiness record costs about an hour of agent time per task; the probes on two real tasks
  found a legal deadline computed wrongly, a false premise and two uncarried checks before any code.
