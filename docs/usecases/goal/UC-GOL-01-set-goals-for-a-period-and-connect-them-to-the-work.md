---
id: UC-GOL-01
title: Set goals for a period and connect them to the work
context: goal
actors: [PE-planner, PE-member, PE-admin, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-11, P-18, P-19]
state: partial
tasks: []
checked_by: [core/domain/model/goal/Goal_test.go, core/application/service/goal/Goal_test.go, test/integration/goal_repository_test.go]
---

# Set goals for a period and connect them to the work

## Goal

A team sets goals for a quarter or a year, at the workspace or in a hub, gives each measurable key
results, hangs goals under the goals they serve, and links the entries that do the work - and the
progress each reader sees is computed from that work and from nothing the reader may not see.

## Story

The workspace's *2026 Q4* holds *Win the market* with the key result *Customers* from 0 to 100, now
50. In the product hub, *Ship the launch* sits under it with two linked entries, one of them done.
*Ship the launch* stands at 50 %; *Win the market* at the mean of its key result and its goal below,
also 50 %. A key result *Tickets solved* fed by a saved summary view of the support queue shows the
view's count for whoever looks.

## How to check

1. A goal belongs to an open period and to the workspace or one hub; creating, changing and
   deleting it needs `WRITE_ITEMS` there, defining and closing a period `STRUCTURE` at the workspace.
2. A key result is a number from a start to a target (either way), a percentage, reached or not, or a
   milestone reached when its entry is completed; its value is reported at a check-in or, for a
   number or a percentage, is the total of a saved summary view as the reader's own query counts it.
3. A goal's progress is the mean of its key results, of the goals under it (a dropped one aside) and
   of the share of its linked entries completed, each counted once; with none of them it has none.
4. Entries linked to a goal are entries the linker may read; the goal's answer lists the linked
   entries the reader may read.
5. A goal in a hub the reader does not read answers not found, and a goal below or a linked entry the
   reader may not read is no part of the progress they see.
6. A goal never sits under itself or a goal of its own (`goals.parent_cycle`), and goals nest at most
   eight deep.
7. Periods, goals, their links and check-ins stay in their workspace and join its archive.
8. REST, MCP and hubctl set, read, link and list goals alike.
9. The web app shows a period's goals as an alignment tree with each goal's progress, and a goal with
   its key results, linked entries and check-ins.

## Where it ends

* No OKR quality hints, goal templates or AI drafting of goals.
* No key result fed by an integration's metric; a view's total is the one automatic source.
* No KPIs kept apart from goals.

## Today

* Check 9: the web app's goal screens are not built (S-21).
