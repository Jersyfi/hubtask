---
id: UC-PLN-04
title: Keep the agreed plan and compare it with today
context: planning
actors: [PE-planner, PE-member, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-11, P-18]
state: built
tasks: []
checked_by: [core/application/service/notification/RecordPlanException_test.go, core/domain/model/planning/Schedule_test.go, core/application/service/planning/Planner_test.go, test/integration/planning_repository_test.go, core/domain/model/planning/Tolerance_test.go, core/application/service/planning/Tolerances_test.go]
---

# Keep the agreed plan and compare it with today

## Goal

A planner keeps the plan as it was agreed under a name and later sees, entry by entry, how far
today's plan has moved from it.

## Story

After the kick-off the planner takes the baseline "Agreed with the board". Six weeks later the
pages finish five days later than agreed, the press kit was added since, and one entry was dropped.
The comparison shows all three.

## How to check

1. A member who may change a plan's entries keeps a baseline of a hub or a collection under a name:
   the start and due of each of its entries at that moment. A plan keeps at most 20
   (`baselines.limit`); a baseline is never changed, only deleted.
2. A comparison lists each entry of the baseline with its agreed and today's start and due and how
   many calendar days each slipped (positive is later), marks the entries added since and those
   gone since, and leaves out every entry the reader may not see.
3. Whoever may read the plan lists its baselines, newest first, with their name, when and by whom
   each was taken, and how many entries it holds.
4. A baseline stays in its workspace: the workspace next door neither reads nor deletes it, and it
   is in the workspace's archive.
5. Whoever may shape a hub or a collection sets its time tolerance, 0 to 365 calendar days; a
   hub's reaches its collections, and a hub that locked it refuses a collection's own; without
   days the tolerance is left to the level above.
6. Reading a plan's tolerance answers its days, the level they come from and the level that
   locked them; once the forecast end lies past the latest baseline's by more than the tolerance,
   the plan is in exception: the *Plan* screen shows it to every reader and the exception is in the
   audit trail (`plan_tolerance.exceeded`).
7. Whoever may shape the workspace sets the tolerance every hub and collection inherits, locked for
   them when asked; it is a block of the workspace's configuration document.
8. A plan found in exception is told by mail to the person who decided its tolerance at the level
   it comes from - never to the reader looking at it - once per baseline; a new baseline may raise
   the next.

## Where it ends

* The Gantt screen's baseline bars and variance columns come with the screen (UC-PLN-02 check 7).
