---
id: UC-TIM-04
title: See where the time went
context: timekeeping
actors: [PE-planner, PE-member, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-14, P-18]
state: built
tasks: []
checked_by: [core/domain/model/timekeeping/Timekeeping_test.go, core/application/service/timekeeping/Keeper_test.go, test/integration/time_repository_test.go, apps/webapp/src/lib/data/time.test.ts]
---

# See where the time went

## Goal

A person sees their own time summed by entry, kind of work or week; a lead sees the team's time
where they approve it, against each entry's estimate - and where the workspace chose aggregated
visibility, only as sums that name nobody.

## Story

The web shop is planned at 120 hours. The report by entry shows "Checkout form" at 9 of its 6
estimated hours, so its remaining hours are 0 and its variance is 3. The works council asked for
aggregated visibility: Clara's report by kind of work shows team sums, and a group two people worked
in is withheld.

## How to check

1. A report sums a range of at most 366 days by up to two of person, entry, collection, hub, kind of
   work, day, week (from Monday), month and billable: minutes, billable minutes and records per
   group; the caller's own time with `mine`, else also other people's where the caller approves
   time, and an entry's title only where the caller may read the entry.
2. A plan's entries the caller may read answer their estimate in hours, everybody's minutes on them,
   both rolled up with the entries below, the hours remaining (never below zero) and the variance.
3. Aggregated, a report over other people takes no person dimension and no other person's filter
   (`time.aggregated_only`), withholds every group of fewer than three people and says how many
   minutes it withheld; other people's records are not listed.
4. Aggregated, only an approver of the plan reads its estimates against actuals.
5. REST, MCP and hubctl read a report and a plan's actuals alike.
6. The web app shows a report and a plan's estimates against actuals as tables.

## Where it ends

* Nothing scores or ranks a person (P-14); there is no individual performance analysis.
* A report counts time, not money: rates, costs and budgets are UC-TIM-08's.
