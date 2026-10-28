# Module 6 — Goals, portfolios & insight (`UC-GOL`, `UC-RPT`)

> **Background** — the concept of 2026-10-17; where it differs from [`../products.md`](../products.md),
> that file wins ([`README.md`](./README.md)).

## 1. Goal

Leadership sees whether the work adds up to what was intended: goals with measurable results fed
by the work itself, portfolios of projects with an honest health status, and dashboards whose
numbers agree with every list they summarise.

## 2. Parity

| Capability (market) | Proposed |
|---|---|
| Goals / OKRs: objectives, key results, owners, periods, alignment tree | `Goal(period, owner, parentGoalId?)`, `KeyResult(kind ∈ NUMBER, PERCENT, CURRENCY, BOOLEAN, ITEMS_DONE, target, current, source)` — `source` may be a saved-view query (progress from work) |
| Check-ins, confidence, history | Check-ins with a confidence value and a note; history kept |
| Portfolios: a list of projects with status, owner, dates, progress, custom fields | `Portfolio` over hubs or collections; status (on track / at risk / off track) set by a person, with a status update text |
| Status updates (written, scheduled reminders) | Status-update pages (knowledge module) on a cadence; a rule reminds the owner |
| Risk / issue / decision registers (RAID) | Record types (platform module) with shipped templates and a probability × impact matrix view |
| Programmes, portfolio roadmaps | Roadmap view over a portfolio (planning) |
| Dashboards: widgets (counts, bar, pie, line, table, burndown, burnup, CFD, velocity, created vs resolved, two-dimensional matrix) | `Dashboard(widgets[{kind, query, grouping, measure}])` built on the query DSL; one query engine for lists and charts so numbers agree (P-11) |
| Flow metrics: cycle time, lead time, throughput, WIP age, time in status | From status history (workflow module); percentiles, not only averages |
| Scheduled reports by mail, PDF export | A dashboard sent on a schedule (notification); PDF rendered server-side without a browser dependency — **decision**, else CSV + image |
| Cross-workspace reporting | Never (P-01). Across hubs of one workspace, yes |
| Query language for power users | The query DSL (api-guidelines §3) gets a typed text form in the search box, with completion |

## 3. Use cases (titles)

GOL-01 Set goals for a period and connect them to the work · GOL-02 Check in on a goal and
say how confident we are · GOL-03 Keep a portfolio of projects and its health in one place ·
GOL-04 Write a status update people actually read · GOL-05 Keep a register of risks ·
RPT-01 Put the numbers I watch on a dashboard · RPT-02 See how work flows and where it waits ·
RPT-03 Send a report on a schedule · RPT-04 Ask for exactly the entries I mean, in one line.

## 4. Principles

P-11 (a chart and its list never disagree; a number that is an estimate says so), P-05 (a widget
counts only what the viewer may see — a dashboard shared with a viewer recomputes for them), P-13
(every chart has a table alternative; colour from tokens, rule 15), P-01.
