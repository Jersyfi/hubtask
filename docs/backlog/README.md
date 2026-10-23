# How work runs

From a concept to a closed milestone. `AGENTS.md` says how to work on a single task; this file says
how tasks come to exist, how they move, and when a milestone is done. Every state named here is
recorded in the repository or on GitHub, so that a person, a coding agent or a tool that steers
agents reads the same thing. Whoever does the work, a person or an agent, is a worker here.

## The stages

```text
concept ──cut──▶ milestone (Delivers) ──release──▶ tasks + issues ──▶ build ──▶ merged
                                                                                   │
                            milestone closed ◀── every Delivers check met ◀────────┘
```

| Stage | Recorded in | Who moves it |
|---|---|---|
| Concept | a conversation with the owner; drafts in a branch | the owner, with a worker |
| Cut | `docs/backlog/milestone-<X>.md` with `Delivers:` and tasks | a worker, in a pull request |
| Released | `**Released:** <date>` in the milestone header; GitHub milestone and one issue per task | the owner's word; a worker writes it |
| Built | the merged pull request (`Closes #n`); the use cases' `state:` and `checked_by:` | the owner merges |
| Closed | `**Closed:** <date>` in the header; the file moves to `docs/archive/backlog/` | a worker, once every `Delivers` check is met |

A concept may instead be built whole, without a cut into tasks (§ "A concept built whole").

## Cutting a milestone

A concept is ready to become a milestone when every item below holds. When the owner works out a
concept with a worker, the worker runs this list and says what is still open — before the
concept lands.

1. **What it delivers.** The use cases exist with *Goal*, *How to check* and *Where it ends*, walked
   through the deployments D1–D7. The milestone's `Delivers:` lists the checks it will make true,
   e.g. `UC-PRV-01 (9, 10), UC-LIF-06`.
2. **Principles.** Every principle the use cases serve is checked against its "Broken when" line.
3. **The matrix.** Where access, a lifecycle or a state is touched: door (web app, REST, MCP,
   hubctl, automation, background worker, installation level) × state × kind of account × deployment — each row
   has an answer or a task.
4. **The premise.** What the concept assumes about today's system is checked against the code.
5. **Decisions.** Every open point has its decider; the owner's points are answered (see
   § "Decisions"), the rest are decided and written down.
6. **The rules.** The subject-document sections that change are drafted; an ADR for each decision
   that needs its reasoning kept.
7. **Findings.** Every open `finding` issue of the area is taken in as a task or closed with a
   reason.
8. **Tasks.** Each task names the checks it carries (`**Use cases:**`), its dependencies, and how it
   will be proven (`**Acceptance:**`). Together the tasks carry every `Delivers` check.
9. **The standing rules.** The rules that do not bend (`AGENTS.md`), the principles and
   `docs/architecture/known-traps.md` are read against the milestone as a whole; what they demand
   of it is part of a task.
10. **A look before the build.** For a milestone that changes the user interface, a preview or a
    prototype the owner looks at before the build, when the owner wants one.

## A concept built whole

On the owner's word — "build concept X completely and bring it to main" — a worker builds a whole
concept on one branch, in one pull request, without cutting it into tasks (`AGENTS.md`, "Working
with the owner", concept mode).

- **It still has its use cases.** The worker writes or updates them — *Goal*, *How to check*,
  *Where it ends*, walked through D1–D7 — on the branch, and the pull request's *Use cases* section
  names every check it makes true. The owner's word asks for new ones; a change to an existing
  *Goal*, *How to check* or *Where it ends* still needs the owner's decision (`AGENTS.md`).
- **The milestone file is optional.** Without one there is no `Delivers` line, no task issues and
  no `**Use cases:**` lines; the pull request closes the concept's issue, or says `No issue:` and
  the owner's word.
- **The cut checklist still applies** where it is about the concept (items 1–7, 9, 10): the worker
  runs it and says what is open before the build starts.
- **The bar is a task's:** proven by tests, inside out, the contract first, the subject documents
  and arc42 updated, `make check`, CI on *Ready*, the merge on the owner's word.
- **Too big for one context:** split into pull requests by screen or area, one after another, each
  complete on its own — never by a cross-cutting theme (all migrations, then all endpoints).

## A milestone file

```markdown
# Milestone PH — Privacy and the household

The goal in one paragraph.

**Delivers:** UC-PRV-01 (9, 10), UC-PRV-03 (9–13), UC-LIF-06
**Released:** 2026-10-02

## Decisions

1. … (the owner's answers and the decisions taken while cutting — current, numbered, never renumbered)

---

## PH-01 — A legal hold wins over an erasure

*Depends on: nothing.*

**Use cases:** UC-PRV-03 (9, 10, 11, 12, 13), UC-LIF-06

What is built, in a few sentences.

**Acceptance:** how it is proven.
```

## While a milestone runs

What a milestone delivers is fixed at release; its tasks are not.

- **Tasks move.** A worker may add, split or merge tasks, as long as every task carries checks from
  `Delivers` (`make gate-docs` refuses one that does not). A new task gets the next free number, a
  GitHub issue in the milestone, and the same treatment as every other task.
- **A finding that blocks a `Delivers` check belongs to the milestone** — fixed in the task that
  found it, or as a new task.
- **A finding outside `Delivers`** is fixed where it is found, on the branch (AGENTS.md, "Working
  rules"); only one clearly bigger than a step becomes a `finding` issue for the next cut.
- **Changing `Delivers`** is the owner's decision (a `decision` issue). The answer is written into
  the milestone's `Decisions` and its `Delivers` line.
- **The milestone is done when every `Delivers` check is met** — not when a task list is empty.
  `make gate-docs` refuses `**Closed:**` while a use case still lists one of those checks as unmet in
  its *Today*.

## Tasks

The task text in the milestone file is the source; the issue (label `task`) is its copy. The worker
checks what the task says about today's code against the code before building on it, corrects a
false premise in the same pull request and names it in the description (*Premises*). A question on
the owner's list that the work cannot go on without becomes a `decision` issue, and the pull
request waits on it in draft; any other open point is settled by the option that changes no rule,
gate or use case, named in the pull request. No other pull request is stacked on a task that waits
on the owner; other tasks go on. No record is written before the code; the readiness records of
earlier tasks are snapshots in `docs/archive/ready/`, not read for current work.

After creating a milestone's issues, compare each issue body with its task in the milestone file —
the issue is only a copy, and copies have been created shifted by one.

## Decisions

A question for the owner is a GitHub issue labelled `decision`, opened from its form
(`.github/ISSUE_TEMPLATE/05-decision.yml`, the fields `AGENTS.md` § "Asking" lists). Only items on
the owner's list in `AGENTS.md` qualify, only when the work cannot go on without the answer, and
only after searching for an earlier answer. Otherwise the worker takes the option that changes no
rule, gate or use case, builds on, and names the choice in the pull request; the owner may overrule
it there. A question about an answered decision names the new fact the code showed.

The questions of a cut or a task start are collected and put to the owner in one message. The answer
lands on `main` the same day in a small documentation pull request of its own — one per batch of
questions, never on a task branch — in the principle, use case, subject document or the
milestone's `Decisions`, wherever it governs, and that pull request closes the issues. An answered
issue is never reopened for the same question; a follow-up names the new fact.

## Findings

Something a worker finds next to its work is fixed there, on the branch (`AGENTS.md`, "Working
rules"). Something clearly bigger than a step, or touching an item the owner decides, or found by a
check, a scheduled run or anyone in the community, is a GitHub issue labelled `finding` (bug reports
from the community arrive through the bug form and are labelled the same). It has no milestone of its own: each cut takes the findings of its area in as
tasks or closes them with a reason, and a sweep may collect bounded fixes in a GitHub milestone that
is a bucket rather than a cut — no `Delivers`, no milestone file, each fix closing its own issue. A finding that blocks a running milestone's `Delivers` is taken in at
once (§ "While a milestone runs").

## Contributions from outside

- **A bug fix** is a pull request that closes the bug's issue or says `No issue:`, with its
  description held to the template (`make gate-pr`).
- **A feature** starts as an issue (feature form). It enters the product through a cut, like every
  other piece of work.
- **People working closely on a part of Hubtask** with their own tools take a milestone of their
  own, follow `AGENTS.md`, and put their questions as `decision` issues — visible to everyone.

## For tools that steer the work

Everything a tool needs is readable without asking a worker:

| Question | Where |
|---|---|
| Which milestones run, what they deliver | `docs/backlog/milestone-*.md` headers (`Delivers`, `Released`, `Closed`) |
| Which tasks exist, which are open | GitHub issues labelled `task`, by milestone |
| Is it being built, is it done | the pull request: draft, ready, merged |
| What waits on the owner | open issues labelled `decision` |
| What was found and not yet placed | open issues labelled `finding` |
| Does the work meet its use cases | the use cases' `state:`, `checked_by:`, *Today* |
