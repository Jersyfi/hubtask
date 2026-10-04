---
name: ready-check
description: Write and attack a task's readiness record before any code — premise against the code, every use case check and documented promise assigned, every door × state × kind × deployment, the standing rules (nobody locked out), races, known traps, feasible acceptance, and every decision with its cheapest decider. Use at a milestone cut for every task, at the start of every task's build (before the first code commit), when an ADR is drafted, and whenever a finding during the build is a question rather than a defect. Arguments - a task id (SC-31), a milestone id (SC), or an ADR number.
---

# /ready-check — is this task settled enough to build?

The opposite end of `/usecase-check`. That skill asks after the build whether the work met its goal;
this one asks before the build whether the concept is complete — so that what `/usecase-check`
finds is a slip in the code, not a question nobody asked. Read
[`docs/backlog/ready/README.md`](../../../docs/backlog/ready/README.md) first, completely: it says
why this exists, the rules, and the escape classes.

**The output is a file**, `docs/backlog/ready/<TASK>.md`, from
[`TEMPLATE.md`](../../../docs/backlog/ready/TEMPLATE.md) — on `main` with the backlog file at a cut,
refreshed as the first commit of the task's branch. Not a chat summary, not a memory note: the next
session reads the file, and the gate reads it too. **Size it first** (README § "Size"): M takes the
short form, L the full one; work of size S has no record.

## 0. Scope

| Argument | What to check |
|---|---|
| a task id | that task: its text in `docs/backlog/milestone-*.md`, its issue, the use cases on its `**Use cases:**` line, the ADRs and documents it names |
| a milestone id | every task of it, one record each, **plus** the milestone-wide coverage of § 2 (every check of every use case the milestone names, assigned to exactly one task or to *Where it ends*). Fan the tasks out to subagents with this skill's steps |
| an ADR number | the ADR as if it were a task: §§ 1, 3, 4, 5 and 8 — before it is put to the owner as `accepted` |

An existing record is **refreshed**, not trusted: § 1 against `main` as it is now, § 8 still valid.

## 1. Read first — all of it, not selectively

* The task text and its issue (with comments). The use cases it names — *Goal*, *How to check*,
  *Where it ends*, *Today* — and `docs/vision/principles.md`, `docs/vision/non-goals.md`.
* The ADRs and subject documents the task names, and the ones they cite for the same area.
* CLAUDE.md's rules that do not bend, and [`known-traps.md`](../../../docs/architecture/known-traps.md).
* The milestone file's header (decisions taken at the cut), its `## Open decisions`, and
  [`standing-decisions.md`](../../../docs/backlog/standing-decisions.md) — every one of them.
* The § 8 of the other records in `docs/backlog/ready/` that touch the same area: what was decided
  there is not decided again.
* The code the task changes **and the code around it**: every caller of what changes, every
  writer of the state it reads.

## 2. Fill the record, section by section

**§ 1 Premise.** List every statement about today's system in the task, its ADRs and the *Today*
sections. Check each against the code with evidence. Grep for "nothing does X", "only", "never",
every quoted number, every named function, action or check number. A false premise is corrected in
its document — it is the most expensive mistake there is, because everything built on it is wrong.

**§ 2 Coverage.** Every check of every named use case, every documented promise the change touches
(api-guidelines, versioning-release, audit, data-protection, retention, offline-sync,
backup-restore). Each carried by this task, another named task, *Where it ends*, or a later milestone
that exists. A promise of "later" without a task is cut as a task now.

**§ 3 Doors and states.** Build the matrix; do not describe it in prose. First **list the
dimensions and their values** (every door, every state, every kind, the deployments that differ,
every failure) — then write the rows, collapsed to the combinations where the outcome differs. A
literal cross product is thousands of cells; a collapse without the list is where a kind or a state
silently goes missing. **Every value of the list appears in at least one row**, or the record says
why it cannot matter. Rows: every door × every
state × every kind of actor, and the deployments D1–D7 where they differ (no mail server, one
workspace, many). Include the lifecycle the entity can go through (delete, restore, import, pending
deletion, suspension) and the failure of every dependency it touches. Each cell: what happens, and
the test or walk that will prove it. An empty cell is a decision (§ 8).

**§ 4 Standing rules.** One line per rule. Apply the owner's no-lockout rule as a search, not a
reading: for every state × kind in § 3 that a change could close, show the way in that remains.

**§ 5 Concurrency.** Every read-then-write and check-then-act; every write on a refusal path (a
write inside the transaction that rolls back is lost — known trap); every counter two requests move.

**§ 6 Known traps.** The entries that apply, and how this task avoids each.

**§ 7 Acceptance.** For each criterion, the test that will prove it and whether that test can
exist (a door that cannot be driven against the database is said so now, not discovered later).

**§ 8 Decisions.** Every choice the task leaves open, every empty cell of § 3, every rule of § 4 that
needs an answer. Give each its **cheapest decider** (README rule 2), in order: a document, a
precedent in the code, an earlier owner decision (search `standing-decisions.md` and every record's
§ 8 first), a worker decision with its reason and rejected alternative — and the owner **only** for
what is on the owner's list. A worker decision is taken, not asked; a session that sends the owner
something off his list wastes his time as surely as one that decides something on it. Owner
decisions are written in the form of [`/decision`](../decision/SKILL.md). **A proposal that promises
a capability is checked against the code before it is written** — a lever that does not exist is a
false premise in a proposal.

## 3. Attack it — the independent review

Hand the record, the task text and the list of documents to a **fresh subagent** that did not write
it, with this brief: *"Find what this readiness record misses: a premise that is false in the code, a
use case check or documented promise not assigned, a door, state, kind, deployment or failure not in
the matrix, a standing rule not answered, a race, an acceptance that cannot be proven, a decision
taken silently that rule 2 says belongs to the owner. Cite file:line or the document sentence for
each. Do not propose code."* Answer every finding in § 9 — by changing the record, not by arguing.

**A second round, when the first changed a lot.** If answering the review added rows to § 3, moved a
premise in § 1 from true to false, or put a new item on the owner's list in § 8, run a fresh reviewer
once more over the changed sections. Two rounds at most: what a second round still finds is answered
in § 9, and anything on the owner's list among it is an open decision — it is not a third round.

## 4. Verdict

* `ready` — every section filled, § 8 without `- [ ]`, § 9 answered.
* `waiting on the owner` — only owner decisions are open. Add them to the milestone file's
  `## Open decisions` and move on to the next task that is not blocked. The owner gets the list as
  **one message** in the `/decision` form — at the cut, or during the build when no unblocked task is
  left. This task is not built until they are answered.
* `not ready` — anything else. Finish the record.

When the owner answers, write the answer into § 8 at once (date, his words in short, where), move
the verdict, and only then start the code. If he reverses a decision later, that is a new line in
§ 8, not an edit of the old one.

## 4a. During the build

A finding during or after the build — including every "not met" of `/usecase-check` — is classified
(README § "Escape classes") and entered in § 10 with the row that should have caught it. D and P are
handled on the branch; C within this task's use cases too. G goes back through § 3 and gets its
cheapest decider — **only an item on the owner's list blocks**; everything else is decided and
built. An open owner decision moves the verdict back to `waiting on the owner`, which holds the
pull request's description gate red until it is answered. N, and C outside this task's use cases,
go to the milestone's `## Inbox`, never into this branch. R is handled like D or O and not counted.

## 5. At the end of a milestone

Every milestone's cut includes a last task, *Escape review*, with its issue — so this step has an
owner and a date. It reads § 10 of every record of the milestone together. Every G, C and P escape
is a question this skill did not ask: add it to this file (§ 2) or to `known-traps.md`. It fills the
milestone file's `## Escapes` table (class → count → what was added where), lists the milestone's
worker decisions for the owner to glance over, and reports the counts next to the milestone's
`/usecase-check`. R escapes are listed and not counted.

Report in the language the owner used.
