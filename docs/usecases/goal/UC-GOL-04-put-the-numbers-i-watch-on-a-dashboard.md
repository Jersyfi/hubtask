---
id: UC-GOL-04
title: Put the numbers I watch on a dashboard
context: goal
actors: [PE-planner, PE-member, PE-admin, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-11, P-13, P-18]
state: built
tasks: []
checked_by: [core/domain/model/goal/Goal_test.go, core/domain/model/view/Aggregate_test.go, core/application/service/work/AggregateItems_test.go, infrastructure/postgres/query/Aggregate_test.go, test/integration/query_aggregate_test.go, core/domain/model/goal/Digest_test.go, core/application/service/goal/Goal_test.go, core/application/service/goal/Digests_test.go, test/integration/goal_repository_test.go, apps/webapp/src/lib/data/goals.test.ts, apps/webapp/src/lib/data/xlsx.test.ts]
---

# Put the numbers I watch on a dashboard

## Goal

A person puts the summaries they watch - counts per status, created against resolved week by week,
lead time, goals' progress - on a dashboard of their own or shares it with a hub or the workspace,
and every reader sees the numbers their own reading allows, the same numbers the lists show.

## Story

The admin builds *Team* at the workspace: a pie of *Launch* by completion and the count of *Merger*,
and shares it. Ben, who may not read the board's hub, opens it: the pie draws, and the *Merger* tile
says he may not see it. Ana's own dashboard *Mine* is hers alone.

## How to check

1. A widget runs a saved summary view, a `POST /items:aggregate` document held to that grammar at
   write, or goals' progress, drawn as a number, a bar, a pie, a line, a table, a matrix or progress on
   a twelve-column grid of at most 24 widgets.
2. A widget's numbers are never stored: reading a dashboard's figures runs each widget now, for the
   reader, as the summary's own use cases answer them.
3. A private dashboard is its owner's alone; one shared with its place is read by everybody who reads
   the workspace or the hub, and sharing one, or changing somebody else's, needs `STRUCTURE` there.
4. A widget the reader cannot run carries its error code in place, and the others still draw.
5. A dashboard the reader does not see answers not found.
6. Dashboards stay in their workspace and join its archive.
7. REST, MCP and hubctl keep dashboards and read their figures alike.
8. The web app draws a dashboard's widgets with design-system colours and a table beside each chart,
   and edits them on the grid.
9. A reader has a dashboard they see sent to them by mail, every day or on one weekday at an hour of
   their zone; a second choice replaces the first, and a dashboard they do not see takes none.
10. The mail carries a link and the total of each number widget computed as the subscriber when it
    is sent - a widget they cannot compute says so -, names neither the dashboard nor a widget
    where their mail lets no title travel; nothing is sent of a dashboard gone or no longer seen,
    and the schedule ends with the subscriber's account.
11. A widget's summary measures cycle time - the first entry into an in-progress status to
    completion -, WIP age and time in status of the open entries from the status history, which
    is written from then on whichever door sets a status; an entry without history holds none.
12. A widget's summary reads a measure's median, 85th or 95th percentile beside its mean, and the
    digest writes each total with the subscriber's decimal separator.
13. A widget's table, and a portfolio's, is handed over as CSV or as a spreadsheet, in which a
    number stays a number and no text runs as a formula.
14. A dashboard shared with groups is read by the members of those groups who read its place - on
    every door and on their devices - and by nobody else but its owner; sharing it needs
    `STRUCTURE` as sharing with the place does.

## Where it ends

* The mail carries no attachment - no CSV and no PDF (the mail port has none by design).
* No wallboard mode, no historical snapshots, no read API for BI tools.
* Entries from before the status history have no cycle time, WIP age or time in status.
* No numbers across workspaces (P-01).
