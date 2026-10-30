---
id: UC-PLN-07
title: Estimate work and see how much a team gets done
context: planning
actors: [PE-planner, PE-member, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-11, P-18]
state: built
tasks: []
checked_by: [core/domain/model/planning/Sprint_test.go, core/application/service/planning/Sprints_test.go, test/integration/sprint_repository_test.go, apps/webapp/src/lib/data/sprints.test.ts, apps/webapp/e2e/sprints.test.mjs]
---

# Estimate work and see how much a team gets done

## Goal

A team says how big each piece of work is, sees day by day how much of a sprint is left against the
ideal line, and knows from its last sprints how much it usually gets done - without keeping the
numbers in a field it has to agree on first.

## Story

The web team estimates in T-shirt sizes. "Checkout form" is an L, which counts as 5 points. In the
sprint's second week the burndown shows 9 points left against an ideal of 6, and a bug that joined
raised the scope line. The velocity of the last three sprints is 18 points, so the lead plans
"Sprint 15" at about that.

## How to check

1. Whoever may change an entry gives it an estimate in points or hours - at most 10000, with at most
   two decimals (`estimates.value_out_of_range`) - or a T-shirt size from XS to XXL that counts as
   1, 2, 3, 5, 8 or 13 points, and removes it again.
2. A sprint counting points adds up points and sizes; one counting hours adds up hours; an entry
   without an estimate, or in the other unit, adds nothing and still counts as an entry.
3. A running or closed sprint's burndown answers each day from its first to today or its close: the
   estimates of the entries in it not done at the day's end, the scope of every entry in it, and the
   ideal line falling from the commitment to zero over the working days of a business calendar or
   Monday to Friday; an entry the reader may not see is not in it.
4. A collection's velocity answers its latest closed sprints, at most 20, newest first, with what
   each committed and completed, and the average completed of the latest three in the unit the
   newest counted; an estimate stays in its workspace and is in the workspace's archive.
5. The sprint screen shows each entry's estimate and lets a member change it, draws the burndown
   with its ideal line and the velocity of the closed sprints, and is readable as a table.
6. REST, MCP and hubctl set and clear an estimate and read the burndown and the velocity alike.

## Where it ends

* The burndown counts each entry at its estimate now; an estimate changed during the sprint is not
  replayed day by day.
* Capacity per person and forecasting a release date from the velocity are not built.
