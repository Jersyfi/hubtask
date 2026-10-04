# The backlog — how work is cut, made ready, built, and how what it finds travels

One milestone is one file, `milestone-<id>.md`: a header with the decisions taken while cutting it,
then one section per task. Every task has a GitHub issue (label `task`, the milestone as its
milestone) whose body is a copy of the task text. Every task has a **readiness record** in
[`ready/`](./ready/README.md) before its first line of code.

Five stages, each with one artefact and one place for its decisions:

| Stage | Artefact | Decisions live in |
|---|---|---|
| 1. Concept | an ADR (`docs/adr/`) for anything architectural | the ADR |
| 2. Cut | `milestone-<id>.md` and the issues | the milestone header |
| 3. Ready | `ready/<TASK>.md`, verdict `ready` | the record's § 8 |
| 4. Build | the branch and the pull request | the record (§ 8 for decisions, § 10 for escapes) |
| 5. Close | `/usecase-check` over the milestone, the escapes read together | `known-traps.md`, `/ready-check` itself |

**Memory is never the only copy.** A session's private notes may point at a decision; the decision
itself is written into one of the places above, the hour it is taken. A rule the owner states for
the whole product goes to him as a proposed line for `docs/vision/` — until he places it, the
milestone header carries it.

## 1. Concept

An ADR is drafted, then run through `/ready-check <ADR>` (premise against the code, doors and states,
standing rules, races, decisions) **before** it is put to the owner as accepted. An ADR that is
accepted within minutes of being written has not been checked against the code; three were revised
within days of acceptance (ADR-0063, ADR-0071's provider rule, ADR-0076).

## 2. Cutting a milestone

1. Write the tasks: each with `*Depends on:*`, `**Use cases:**` (the checks it makes true),
   the text, and `**Acceptance:**`.
2. **Coverage across the milestone**: every check of every use case the milestone names is carried
   by exactly one task, or by *Where it ends*, or by a later milestone that exists. A check nobody
   carries is a task, not a hope for the final walk.
3. `/ready-check <milestone>`: one record per task, attacked by an independent reviewer.
4. **One decision round with the owner** for the whole cut: every open decision of every record —
   only what is on his list (`ready/README.md` rule 2) — in one message, in the form of
   [`/decision`](../../.claude/skills/decision/SKILL.md). His answers go into the records, and into
   [`standing-decisions.md`](./standing-decisions.md) when they hold beyond the task.
5. The backlog file and **every record, whatever its verdict,** land on `main` in the cut's
   docs-only pull request — so an open question is on `main`, not on a parked branch. Its commits
   carry `Milestone: <id>`, not `Task:` trailers: a cut is not a task's build.
6. Only then the issues. A task whose record still waits on the owner gets its issue with the
   label `needs-decision`, and is not started.
7. The milestone file ends with three sections: `## Open decisions` (what waits on the owner,
   one line each, pointing at the record), `## Inbox` (new scope found while building, for his
   triage), and `## Escapes` (filled at the close). Its last task is *Escape review*, with an issue.

## 3. Ready — before the first commit of a task

`/ready-check <TASK>` refreshes the record against `main` as it is now. The verdict must be `ready`.
The record is the first commit of the branch; the draft pull request names it. The gate
`Pull request description` refuses a description whose record is missing, not `ready`, or has an
open decision — so a pull request cannot be merged past an unanswered question.

## 4. Build — and findings

Everything that turns up while building or reviewing is classified once, by the escape classes of
[`ready/README.md`](./ready/README.md), and goes exactly one way:

| What it is | Where it goes |
|---|---|
| A defect in this branch's code | fixed on the branch, test first; no issue |
| A defect in older code, fixable within the rules | fixed on the branch in its own commit, entered in the record's § 10; no issue |
| A defect too large for this branch | an issue (label `bug`, the current milestone), entered in § 10 |
| A use case check or promise no task carried | within this task's use cases: the record's § 2, built here; otherwise the milestone's `## Inbox` |
| A gap in the concept — the cheapest decider takes it | the record's § 8, decided by a document, a precedent, an earlier answer or the session (with its reason), then built |
| A decision on the owner's list | the record's § 8 as open; the verdict goes back to `waiting on the owner`; the milestone's `## Open decisions`; other work goes on, and the owner gets the list in one message when nothing unblocked is left |
| A wrong premise in a document | the document is corrected in the same pull request |
| New scope nobody asked for | the milestone's *Inbox* (below) — not built, not an issue yet |

A finding is **never** only a sentence in a pull request description, a comment or a session's
memory. The pull request may report it; the record holds it.

## 5. Closing a milestone

The milestone's last task, *Escape review*: `/usecase-check <milestone>`, and the § 10 escapes of
every record read together. Every G, C and P escape becomes a question in `/ready-check` or an entry
in [`known-traps.md`](../architecture/known-traps.md). The `## Escapes` table gets the count per class
and what was added where; R escapes (real use, reversals) are listed and not counted. The worker
decisions of the milestone are listed for the owner to glance over. The counts are the measure of
whether preparation is getting better.

## The Inbox, and no new round on top of an unfinished one

Each milestone file ends with an `## Inbox` section: new scope and ideas found while building
(class N), one line each, with where it came from. The owner triages it when the milestone closes,
or earlier when he chooses. **A milestone is not given a new round of tasks while its original tasks
are unbuilt**, unless a new task blocks one of them — follow-ups are built inside the task that found
them (§ 4), or wait in the Inbox. That is what keeps a milestone from growing faster than it is built.
