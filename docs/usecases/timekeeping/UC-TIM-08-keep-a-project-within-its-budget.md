---
id: UC-TIM-08
title: Keep a project within its budget
context: timekeeping
actors: [PE-planner, PE-owner, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-14, P-18]
state: built
tasks: []
checked_by: [core/domain/model/timekeeping/Costs_test.go, core/application/service/timekeeping/Resources_test.go, test/integration/time_resources_test.go, apps/webapp/src/lib/data/time.test.ts]
---

# Keep a project within its budget

## Goal

Whoever manages costs gives a hub or a collection a budget in hours, in money or both, prices
people's and roles' hours with rates that hold from a day to a day, and sees the budget burn week by
week - and what a person earns is read by nobody else.

## Story

The web shop has 3 hours and 200.00 EUR. Ana's own rate is 90.00 EUR an hour until 2 November; the
designer role's in the hub is 60.00. An hour of Ana's on 2 November and two on 3 November cost
210.00: the burn shows both budgets passed, and Ben's half hour without a rate as unpriced.

## How to check

1. Rates, budgets and the burn need `MANAGE_COSTS` (or `MANAGE_MEMBERS`) - at the workspace for a
   rate, at the plan for a budget and its burn; anybody else is refused.
2. A rate prices a person or a role with an amount in a currency's minor unit and an ISO 4217 code
   from a day to a day or on; two rates of one subject never share a day (`time.rate_overlaps`).
3. A day's time is priced by the person's own rate, else by the rate of their most specific role
   where the plan lies; a rate in another currency than the budget's is not converted and its
   minutes are unpriced.
4. A plan's burn answers every minute recorded on its entries and their cost against the budget,
   whether either budget is passed, and the cumulative burn week by week.
5. A rate's amount never reaches the audit trail.
6. Rates and budgets stay in their workspace.
7. REST, MCP and hubctl read and write rates, budgets and the burn alike.
8. The web app shows the rates under *How we work*, and a plan's budget with its burn on the plan's
   actuals.

## Where it ends

* No invoices, no exchange rates, no forecast of the end cost.
* A budget is a plan's; there is no portfolio budget across hubs.
