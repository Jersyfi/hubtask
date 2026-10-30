---
id: UC-PLN-05
title: Mark the dates that matter as milestones
context: planning
actors: [PE-planner, PE-member, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-11, P-18]
state: built
tasks: []
checked_by: [core/domain/model/planning/Schedule_test.go, core/application/service/planning/Planner_test.go]
---

# Mark the dates that matter as milestones

## Goal

A planner marks an entry as a milestone: a date without a duration that the work before it meets
and the work after it waits for.

## Story

"Go live" is a milestone after "Build the pages". When the pages finish on Friday, go-live is on
that Friday; the launch campaign waits for go-live and starts on Monday.

## How to check

1. Whoever may change an entry marks it as a milestone, or no longer; the mark is kept when its
   scheduling mode changes, and the other way round.
2. A milestone has no duration: an automatic milestone after a finish-to-start dependency falls on
   the day its predecessor finishes, and what waits for it starts on the next working day.
3. Reading a plan answers each milestone with one date and `milestone: true`.

## Where it ends

* A milestone is a mark on an entry of any type; a workspace's own milestone type is a type with
  the mark set by its template.
