# Module 2 — Planning (`UC-PLN`)

> **Background** — the concept of 2026-10-17; where it differs from [`../products.md`](../products.md),
> that file wins ([`README.md`](./README.md)).

## 1. Goal

A planner sees what depends on what, which chain decides the end date, how the plan compares with
the one agreed, and what a team commits to in the next two weeks — and a member sees only the dates
that concern them.

## 2. Parity

| Capability (market) | Hubtask today | Proposed |
|---|---|---|
| Dependencies FS / SS / FF / SF with lag and lead | none; "not planned" (UC-WRK-03) | `Dependency(predecessorId, successorId, kind, lagDuration)` across collections of one workspace; cycles refused |
| Gantt / timeline with drag, dependency arrows | Timeline layout (`startAt`, `dueAt`) | Gantt layout: bars, arrows, drag with the dependency respected or shown as a conflict |
| Auto-scheduling, critical path | — | Critical path computed (pure domain function); auto-schedule *proposes* moves, a person accepts (P-04 spirit: no silent mass change) |
| Baselines (snapshot, compare) | — | `Baseline(name, takenAt, items[{id, startAt, dueAt}])`, up to N per plan; variance in the Gantt |
| Milestones | mentioned as possible type (`domain-model.md` §2) | `MILESTONE` type at task level: a date, no duration, completion |
| Sprints / cycles: backlog, commit, carry-over, velocity, burndown | — | `Sprint(collectionId, name, startAt, endAt, goal, state ∈ {PLANNED, ACTIVE, CLOSED})`; closing moves unfinished items to the next sprint or backlog, by the person's choice |
| Estimates: story points, hours, T-shirt | custom fields | First-class `estimate {unit ∈ POINTS, HOURS, SIZE, value}` so velocity and capacity compute without guessing a field |
| Releases / versions, release notes | — | `Release(name, date, state)` an entry is in; release notes page generated into the knowledge module |
| Roadmap across teams (initiatives over time) | — | A roadmap view over a portfolio (strategy module) — lanes per hub, bars per epic-type item |
| Workload-aware scheduling | — | Via resources module (capacity) |
| Project templates with relative dates | Templates with relative due dates | Templates carry dependencies and milestones |
| Import of a plan | Spreadsheet import | MS Project XML / generic CSV with predecessors |

## 3. Use cases (titles)

| ID | Title |
|---|---|
| PLN-01 | Say that one piece of work waits for another |
| PLN-02 | See the plan as a Gantt chart and move it by hand |
| PLN-03 | Know which chain decides the end date |
| PLN-04 | Keep the agreed plan and compare it with today |
| PLN-05 | Mark the dates that matter as milestones |
| PLN-06 | Plan a sprint, run it and close it |
| PLN-07 | Estimate work and see how much a team gets done |
| PLN-08 | Collect work into a release and tell people what is in it |
| PLN-09 | Bring a plan in from a project file |

## 4. Principles, rules, where it ends

P-04 (auto-scheduling proposes, never moves many entries silently), P-11 (a date that cannot hold
says so), P-13 (Gantt keyboard-operable and readable as a table — accessibility by number, WCAG
2.2 AA; a Gantt without a table alternative fails), P-12. Rule 4 (critical path uses the `Clock`
port), rule 7 (scheduling bounded in time and size).

Ends: no resource levelling across workspaces; no earned-value accounting (resources module has
cost, not EVM — candidate for later); no dependencies across workspaces (P-01).
