---
id: UC-TIM-05
title: Hand time to accounting
context: timekeeping
actors: [PE-admin, PE-integrator, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-07, P-18]
state: built
tasks: []
checked_by: [core/domain/model/timekeeping/Timekeeping_test.go, core/application/service/timekeeping/Keeper_test.go, test/integration/time_repository_test.go]
---

# Hand time to accounting

## Goal

At the month's end the workspace hands its approved time to accounting as rows it can import,
without Hubtask making an invoice.

## Story

The office manager exports November's billable time of the web shop hub: 412 rows with day, person,
entry, kind of work, hours and note, which the accounting program reads.

## How to check

1. An export answers at most 50000 records of a range of days in fixed columns - record, day,
   person, hub, collection, entry and its title, kind of work, minutes, hours, billable, source,
   corrected record, note - and says whether it was cut short.
2. It holds the caller's own records with `mine`, else also other people's where the caller
   approves time; aggregated, the caller's own only (`time.aggregated_only`).
3. Every export is recorded in the audit trail with its range and its number of rows.
4. REST, MCP and hubctl export alike.

## Where it ends

* No invoice is made (NG-invoicing); no payroll.
* A file in a particular accounting program's format is not built; the rows are the hand-over.
