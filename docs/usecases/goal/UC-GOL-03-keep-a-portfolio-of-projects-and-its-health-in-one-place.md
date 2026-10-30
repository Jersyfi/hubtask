---
id: UC-GOL-03
title: Keep a portfolio of projects and its health in one place
context: goal
actors: [PE-planner, PE-owner, PE-admin, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-11, P-18]
state: built
tasks: []
checked_by: [core/domain/model/goal/Goal_test.go, core/application/service/goal/Goal_test.go, test/integration/goal_repository_test.go, apps/webapp/src/lib/data/goals.test.ts]
---

# Keep a portfolio of projects and its health in one place

## Goal

Leadership keeps hubs and collections together as a portfolio with an owner, dates and a health per
project, sees each project's progress as a table that agrees with the project's own list, nests
portfolios, and writes status updates whose health becomes the portfolio's.

## Story

*Programme* holds the *Launch* collection, on track from October to December, and the *Merger*
collection. Ana reads *Launch* with 4 entries, 3 completed, and the programme at 75 %. Ben, who may
not read the board's hub, sees only *Launch*; when he changes the programme, *Merger* stays in it.
Ana's update "late, at risk" makes the programme at risk.

## How to check

1. A portfolio holds at most 200 hubs and collections, each once, with optional dates and a health;
   creating, changing and deleting it needs `WRITE_ITEMS` at the workspace, and a project added is one
   the writer may read.
2. Read alone, each project shows how many entries it holds and how many are completed, as
   `POST /items:aggregate` counts them for the reader, and the portfolio its progress over them.
3. A project the reader may not read is not shown, and a writer who replaces the projects keeps the
   ones they may not read.
4. Portfolios nest at most six deep, never inside themselves; deleting one moves its portfolios up to
   its parent and leaves its hubs and collections as they are.
5. A status update gives on track, at risk or off track and a summary; it is kept as history and its
   health becomes the portfolio's.
6. Portfolios, their projects and updates stay in their workspace and join its archive.
7. REST, MCP and hubctl keep portfolios and their updates alike.
8. The web app shows a portfolio as a table with each project's dates, health and progress, and its
   updates.
9. For a reader who manages costs at a project (`MANAGE_COSTS`, or `MANAGE_MEMBERS`), the portfolio
   read alone shows the project's budget and burn as the time module answers them, and their sum -
   money only while every budget is in one currency; nobody else sees a budget.

## Where it ends

* No portfolio timeline or workload view.
* No scheduled status reports and no intake of proposals into projects.
