---
id: UC-GOL-04
title: Put the numbers I watch on a dashboard
context: goal
actors: [PE-planner, PE-member, PE-admin, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-11, P-13, P-18]
state: partial
tasks: []
checked_by: [core/domain/model/goal/Goal_test.go, core/application/service/goal/Goal_test.go, test/integration/goal_repository_test.go]
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

## Where it ends

* No scheduled report by mail and no PDF yet.
* No wallboard mode, no historical snapshots, no read API for BI tools.
* No numbers across workspaces (P-01).

## Today

* Check 8: the web app's dashboards are not built (S-21).
