---
id: UC-PLN-02
title: See the plan as a Gantt chart and move it by hand
context: planning
actors: [PE-planner, PE-member, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-04, P-08, P-11, P-13, P-18]
state: built
tasks: []
checked_by: [core/domain/model/planning/Schedule_test.go, core/application/service/planning/Planner_test.go, test/integration/planning_repository_test.go, apps/webapp/src/lib/data/planning.test.ts, apps/webapp/src/lib/data/schedule.test.ts]
---

# See the plan as a Gantt chart and move it by hand

## Goal

A planner sees a hub's or a collection's entries as bars on a time line with their dependencies,
says per entry whether the plan may move it, and moves what may be moved along the dependencies -
never more than they accepted, and never an entry somebody keeps by hand.

## Story

The relaunch plan in the web hub: "Approve the design" ends on Wednesday, "Build the pages" waits
for it finish-to-start and is set to automatic, "Go live" is a milestone after it. The planner
opens the plan; "Build the pages" is still drawn three weeks out, and the plan proposes Thursday
and Friday. The planner accepts the move. The copywriting entry, kept by hand, starts before the
design is approved; the plan names that dependency as broken and leaves the entry where it is.
The design lead sets the copy to start two days after the design starts instead.

## How to check

1. A dependency is finish-to-start, start-to-start, finish-to-finish or start-to-finish, with a lag
   or a lead of up to 999 working days; whoever may change the waiting entry sets it. A link nobody
   waits on takes none (`links.timing_without_dependency`), a lag past the bound is refused with
   `planning.lag_out_of_range`.
2. Every entry is manual until a planner says otherwise; an entry set to automatic is placed as
   early as its predecessors allow, on the working days of a business calendar the plan names, or
   Monday to Friday, its duration in working days kept; a finished entry never moves.
3. Reading the plan answers every entry the reader may see with its start, finish, duration and
   whether the plan proposes to move it; a manual or finished entry a dependency cannot hold is
   named with the earliest date the dependency allows, and stays where it is; an entry the reader
   may not see, and every dependency with an end out of reach, is not in the answer.
4. Applying the plan moves only the entries the person accepted, all or none, each as a change of
   its start and all-day due date that is checked, recorded and announced as one made by hand; the
   plan is scheduled again before it moves anything.
5. A parent set to automatic spans its children; a dependency to or from it is drawn and not
   scheduled.
6. A loop of dependencies cannot be scheduled and is refused with `planning.cycle`.
7. The Gantt screen draws the plan's bars, milestones and summary bars, a today line and zoom from
   a day to a year, moves and resizes a bar by dragging and draws a dependency by dragging, and is
   operable by keyboard and readable as a table.
8. REST, MCP and hubctl read the plan, apply it, and set a dependency's timing and an entry's mode
   alike.

## Where it ends

* Constraint types (start no earlier than, must finish on), resource levelling and probabilistic
  dates are not built.
* A plan is one hub or one collection; an entry outside it that a dependency names is shown and
  kept where it is, never moved by this plan.
* Export of the chart to PDF or PNG and of the plan to a spreadsheet is the release after M2's first.
