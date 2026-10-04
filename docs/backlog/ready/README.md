# Readiness records — what is settled before the first line of code

A task is built only when its **readiness record** says `ready`. The record is the task's concept,
written down and attacked before anything is coded: what is true today, which checks and promises
the task covers, every door and state it touches, every standing rule, every race, every decision —
and who took each decision. One file per task, `docs/backlog/ready/<TASK>.md`, from
[`TEMPLATE.md`](./TEMPLATE.md), written by [`/ready-check`](../../../.claude/skills/ready-check/SKILL.md).

## Why this exists

Measured on 2026-10-04 over the milestones SC, SI and F8–F10, and over the session that built
SC-16…SC-30: of 47 open questions and findings in that session, **31 were answered by a sentence in a
document that existed before the code was written**, and 11 more partly. They surfaced only after
the build, in `/usecase-check`, as new issues, new owner questions and new backlog rounds — SC grew
from 15 tasks to 30 in four days while 11 of its original tasks stayed unbuilt. The causes, in order
of how many follow-ups each explains:

1. **The premise was never checked against the code.** *Today* sections, ADR contexts and task
   texts were trusted; three of them claimed more than the code did.
2. **No state × door matrix** — which states the entity can be in, which doors reach it.
3. **The owner's standing rules were not applied as a check** — the no-lockout question came back
   five times.
4. **No concurrency and transaction pass** — every race was found by the reviewer.
5. **Acceptance criteria and "later" promises written without checking they can be met.**
6. **The project's own rules and known traps not read first.**

And on the side of the documents: concepts (ADRs and task texts) were written and accepted within
minutes, and the only systematic check ran *after* the build. The record moves that check before it.

What the record does **not** fix, said so that nobody expects it: defects already on `main` that only
real use reveals, and the owner changing his mind after seeing a built thing. About half of the
follow-ups of F8–F10 were of these two kinds. They are findings, class R below, and are not counted
against preparation.

## The rules

1. **No code before `ready`.** A commit that changes anything outside `docs/` may only follow a
   commit in which the task's record says `**Verdict:** ready`. The gate `Pull request description`
   (`tools/checkpr`) reads the branch's history and refuses a pull request whose first code commit
   came before that, whose record is missing, is not `ready`, names another task, still carries the
   template's placeholders, leaves a section empty, or has an open decision.

2. **Every decision has its decider — the cheapest one that may take it.** In § 8 each line is, in
   this order of preference:
   1. **decided by a document** — cite the file, the section and the sentence;
   2. **decided by precedent** — the same shape already exists in the code; cite `file:line`;
   3. **decided by the owner earlier** — found in [`../standing-decisions.md`](../standing-decisions.md)
      or in another record's § 8; cite it. *A question already answered is never asked again;*
      a session searches both before it classes anything as the owner's;
   4. **worker decision** — anything not on the owner's list below: the session decides, writes the
      choice, the reason and the alternative it rejected. The owner sees every worker decision of
      a milestone at its close and may reverse any; until then it stands;
   5. **owner** — only what is on the owner's list. Open, it is `- [ ]`, and the verdict is
      `waiting on the owner`.

   **The owner's list** is CLAUDE.md "What you do not decide yourself", word for word, plus the
   no-lockout rule: any deviation from an ADR or a subject document; any new third-party dependency;
   a change to `api/openapi.yaml` that renames or removes an existing field; a change to the licence
   model, the security gates or the retention safeguards; anything that could irrecoverably delete
   user data; any change to a use case's *Goal*, *How to check* or *Where it ends*, and anything in
   `docs/vision/`; **and anything that changes *whether* a person can reach the platform and their
   data** (ADR-0077 §4 — *how* they sign in is not on the list). Nothing else is.

3. **The owner is asked in one message, with a proposal.** Every owner question uses the form of
   [`/decision`](../../../.claude/skills/decision/SKILL.md): the problem in plain words, a worked-out
   proposal that fits the use cases, the vision and the deployments, the alternatives and why not,
   and what happens until he answers. Open questions are collected in the milestone file's
   `## Open decisions` list. A session that meets one moves on to the next task that is not blocked;
   the owner gets the list **as one message** — at the cut, and during the build only when no
   unblocked task is left. His answer is written into the record's § 8 and, if it holds beyond the
   task, into `standing-decisions.md`, the same hour.

4. **An independent reviewer attacks the record before the verdict.** A fresh subagent with only the
   record, the task and the documents — not the session that wrote it — looks for a missing door,
   state, rule, race or premise, and for a worker decision that is really on the owner's list. Its
   findings are answered in the record. The same independence is why `/usecase-check` finds things;
   it now runs twice, before and after.

5. **What escapes is recorded where it escaped.** A finding during or after the build goes into § 10
   with its class and the row of § 1–7 that should have caught it. At a milestone's close the escapes
   are read together, and every G, C and P escape becomes a question in `/ready-check` or a line in
   [`known-traps.md`](../../architecture/known-traps.md). That is how the check gets better instead of
   the backlog getting longer.

## Size — the record is proportional

| Size | What it is | Record |
|---|---|---|
| S | documentation, CI, tooling; serves no use case; carries no `Task:` trailer; touches nothing on the owner's list and none of `api/openapi.yaml`, `db/migrations/`, dependency manifests, `docs/adr/`, `docs/usecases/` | none — the description's *Readiness* says `n/a — <reason>` |
| M | a defect that restores documented behaviour: no new door, state, contract, migration or dependency; about eight files at most | the short form: header, §§ 1, 4, 7, 8, 10 filled; the others say `none — size M: <why>`; the reviewer may be skipped |
| L | everything else | the full record, with the reviewer |

The gate refuses Size M when the branch changes the contract, the schema, a dependency or an ADR,
and refuses n/a when the commits carry a `Task:` trailer or change any of those or a use case.

## When it is written

* **At the cut** of a milestone, for every task, before the issues are created
  ([`../README.md`](../README.md) § 2). The records land on `main` with the backlog file — whatever
  their verdict, so that an open question is on `main` and not on a parked branch.
* **At the start of the build**, refreshed: the premise against `main` as it is now, the decisions
  still valid. If nothing changed, the draft pull request says so; if something did, the refresh is
  the branch's first commit.
* **A task cut before 2026-10-04** gets its record at the start of its build, the same way.
  Pull requests opened before that day may answer `n/a — opened before the readiness gate`; the
  gate reads their opening date.

## What it costs the owner, honestly

One decision message per milestone cut, holding only items on his list, each with a proposal. During
a build, a question reaches him only for an escape on his list, and only when nothing unblocked is
left — collected, not one by one. At the milestone's close, a list of the worker decisions taken, to
glance over. And, as before, his word on each merge. Fewer questions than before, asked earlier.

## Escape classes

| Class | Meaning | Route |
|---|---|---|
| D | defect in code — this diff or older | fix it on the branch, own commit, test first |
| G | concept gap — a door, state, kind, deployment or failure nobody considered | add it to § 3, decide it by rule 2, then build it here |
| C | coverage gap — a use case check or documented promise no task carried | within this task's use cases: build it here; otherwise the milestone's `## Inbox` |
| O | owner decision the record left open | § 8 as `- [ ]`, verdict back to `waiting on the owner`, `## Open decisions` |
| P | premise wrong — a task, ADR or *Today* says what the code does not | correct the document in the same pull request |
| N | new scope — something nobody asked for before | the milestone's `## Inbox`; the owner triages it; not built here |
| R | real use or reversal — an old defect found by using the product for real, or the owner changing a decision after seeing the build | handled like D or O; **not counted against preparation** |
