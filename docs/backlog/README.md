# How work runs

From a concept to a closed milestone. `AGENTS.md` says how to work on a single task; this file says
how tasks come to exist, how they move, and when a milestone is done. Every state named here is
recorded in the repository or on GitHub, so that a person, a coding agent or a tool that steers
agents reads the same thing. Whoever does the work, a person or an agent, is a worker here.

## The stages

```text
concept ──cut──▶ milestone (Delivers) ──release──▶ tasks + issues ──▶ readiness ──▶ build ──▶ merged
                                                                                                │
                                         milestone closed ◀── every Delivers check met ◀────────┘
```

| Stage | Recorded in | Who moves it |
|---|---|---|
| Concept | a conversation with the owner; drafts in a branch | the owner, with a worker |
| Cut | `docs/backlog/milestone-<X>.md` with `Delivers:` and tasks | a worker, in a pull request |
| Released | `**Released:** <date>` in the milestone header; GitHub milestone and one issue per task | the owner's word; a worker writes it |
| Ready | `docs/backlog/ready/<TASK>.md`, `**Verdict:** ready` | a worker, first commit of the task's branch |
| Built | the merged pull request (`Closes #n`); the use cases' `state:` and `checked_by:` | the owner merges |
| Closed | `**Closed:** <date>` in the header; the file moves to `docs/archive/backlog/` | a worker, once every `Delivers` check is met |

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

- **Tasks move.** A session may add, split or merge tasks, as long as every task carries checks from
  `Delivers` (`make gate-docs` refuses one that does not). A new task gets the next free number, a
  GitHub issue in the milestone, and the same treatment as every other task.
- **A finding that blocks a `Delivers` check belongs to the milestone** — fixed in the task that
  found it, or as a new task.
- **A finding outside `Delivers`** is fixed on the branch where the rules allow (AGENTS.md, "Working
  rules"); otherwise it becomes a `finding` issue for the next cut.
- **Changing `Delivers`** is the owner's decision (a `decision` issue). The answer is written into
  the milestone's `Decisions` and its `Delivers` line.
- **The milestone is done when every `Delivers` check is met** — not when a task list is empty.
  `make gate-docs` refuses `**Closed:**` while a use case still lists one of those checks as unmet in
  its *Today*.

## Tasks

The task text in the milestone file is the source; the issue (label `task`) is its copy. Before the
first line of code a worker writes the readiness record from `ready/TEMPLATE.md` against the code
as it is that day, has it attacked by a reviewer who did not write it, and commits it as the
branch's first commit. Its verdict is `ready`, or `waiting on the owner` naming the open `decision`
issues — code that does not depend on them may start, but the pull request leaves draft only when
the verdict is `ready`. After the merge the record is a snapshot: nobody updates it. No other pull
request is stacked on a task that waits on the owner; other tasks go on.

After creating a milestone's issues, compare each issue body with its task in the milestone file —
the issue is only a copy, and copies have been created shifted by one.

## Decisions

A question for the owner is a GitHub issue labelled `decision`, opened from its form
(`.github/ISSUE_TEMPLATE/05-decision.yml`, the fields `AGENTS.md` § "Asking" lists). Only items on
the owner's list in `AGENTS.md` qualify, and only after searching for an earlier answer. Questions
are asked at the cut and at a task's start; during the build only when the code refutes a decision,
and the question names that new fact.

The questions of a cut or a task start are collected and put to the owner in one message. The answer
lands on `main` the same day in a small documentation pull request of its own — one per batch of
questions, never on a task branch — in the principle, use case, subject document or the
milestone's `Decisions`, wherever it governs, and that pull request closes the issues. An answered
issue is never reopened for the same question; a follow-up names the new fact.

## Findings

Something found outside a task, by a worker, a check, a scheduled run or anyone in the community,
is a GitHub issue labelled `finding` (bug reports from the community arrive through the bug form and
are labelled the same). It has no milestone. Each cut takes the findings of its area in as tasks or
closes them with a reason. A finding that blocks a running milestone's `Delivers` is taken in at
once (§ "While a milestone runs").

## Contributions from outside

- **A bug fix** is a pull request that closes the bug's issue or says `No issue:`; it has no
  readiness record. Where it touches the contract, a migration or a query, a dependency, an ADR or
  what a use case promises, it says `Readiness: n/a — <why>` instead (`make gate-pr`).
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
| Is a task ready | `docs/backlog/ready/<TASK>.md`, `**Verdict:**` |
| Is it being built, is it done | the pull request: draft, ready, merged |
| What waits on the owner | open issues labelled `decision` |
| What was found and not yet placed | open issues labelled `finding` |
| Does the work meet its use cases | the use cases' `state:`, `checked_by:`, *Today* |
