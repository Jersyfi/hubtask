---
id: UC-PLN-03
title: Know which chain decides the end date
context: planning
actors: [PE-planner, PE-member, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-11, P-18]
state: built
tasks: []
checked_by: [core/domain/model/planning/Schedule_test.go, core/application/service/planning/Planner_test.go]
---

# Know which chain decides the end date

## Goal

A planner sees which entries decide when the plan ends, and how far every other entry may slip
before it moves the end or the next entry.

## Story

The relaunch ends on the ninth. The chain design → pages → go-live is critical; the press kit runs
beside the pages and has a day of float. When the planner asks why the end date is the ninth, the
answer is the chain, earliest first.

## How to check

1. Reading a plan answers its last working day and its critical path: the entries with no float,
   earliest first.
2. Every scheduled entry carries its total float - how many working days it may slip before the
   plan's end slips - and its free float - before a successor must move - counted on the plan's
   calendar; a chain a manual entry breaks shows negative float.
3. The same plan on the same calendar always answers the same dates and the same path.

## Where it ends

* Float is counted in whole working days; hours within a day are not planned.
* Earned value and probabilistic finish dates are not built.
