---
id: UC-TIM-06
title: See who has room before handing out work
context: timekeeping
actors: [PE-planner, PE-member, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-14, P-18]
state: built
tasks: []
checked_by: [core/domain/model/timekeeping/Resources_test.go, core/application/service/timekeeping/Resources_test.go, test/integration/time_resources_test.go, apps/webapp/src/lib/data/time.test.ts]
---

# See who has room before handing out work

## Goal

Before a lead hands out work in a hub or a collection, they see each person's weeks: what the person
can give and what their open entries already ask of it, with an overload marked - and where the
workspace chose aggregated visibility, a reader who does not approve time sees only the team's sums.

## Story

Ana works four days of six hours. Her checkout form, estimated at 4 hours with 1 recorded, is due
this Friday, and she is on leave that Friday. The web shop's workload shows her week at 180 of 240
minutes; the lead's week is empty, so the next entry goes to him.

## How to check

1. A person writes their own capacity - the minutes of each weekday, Monday first, from a day on
   until their next capacity begins; another person's capacity needs `MANAGE_MEMBERS` at the
   workspace, and reading it `APPROVE_TIME` or `MANAGE_MEMBERS`.
2. A plan's workload answers, per person and week from a Monday, up to 26 weeks, the available
   minutes - capacity, nothing on a business calendar's holiday, nothing or half on an absence - and
   the minutes their assigned open entries' remaining estimates in hours ask, spread over the days
   each entry is scheduled on; a week asking more than the person can give is marked over, one
   leaving a fifth unasked under.
3. An entry the reader may not read, a done entry, and one without an hours estimate ask nothing;
   an assigned open entry with neither a start nor a due day is counted as unscheduled.
4. Aggregated, a reader without `APPROVE_TIME` or `MANAGE_MEMBERS` at the plan reads only the team's
   weeks - available and allocated minutes and how many people - naming nobody.
5. Capacities stay in their workspace.
6. REST, MCP and hubctl read the workload and write a capacity alike.
7. The web app shows a plan's workload as a heatmap of people by weeks, and a person writes their
   capacity on *My time*.

## Where it ends

* Nothing assigns or levels work by itself; the lead decides.
* Nothing scores or ranks a person (P-14): the workload compares minutes, not output.
