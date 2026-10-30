---
id: UC-TIM-03
title: Approve a team's time
context: timekeeping
actors: [PE-admin, PE-planner, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-14, P-18]
state: built
tasks: []
checked_by: [core/domain/model/timekeeping/Timekeeping_test.go, core/application/service/timekeeping/Keeper_test.go, test/integration/time_repository_test.go, apps/webapp/src/lib/data/time.test.ts]
---

# Approve a team's time

## Goal

A project lead approves the weeks of the people who worked on their project, or sends one back with
a reason, and the workspace locks a period so that approved and closed time no longer moves -
corrections go into an open week instead.

## Story

Clara leads the web shop hub and holds a role with APPROVE_TIME there. Her queue shows Ana's week;
Tuesday is missing, so she sends it back. Ana adds Tuesday and hands it in again; Clara approves it.
At the month's end the administrator locks October. In November Ana finds 30 minutes too many in an
approved week and records a correction of -30 minutes in the open week.

## How to check

1. Whoever holds APPROVE_TIME - or MANAGE_MEMBERS, which the shipped owner and administrator hold -
   at the workspace, or at every hub or collection a week's entries lie in, sees that week in the
   queue, reads it and approves it; anybody else does not see it in the queue and is refused.
2. Nobody approves or sends back their own week (`timesheets.own_week`); sending back needs a
   reason of at most 500 characters, which stays out of the audit trail.
3. An approved record never changes; a correction is a new record of signed minutes on a day of an
   open week, naming the record it corrects, and a record that may still change answers
   `time.correction_not_needed`.
4. Whoever approves time at the workspace sets the last locked day; no record on or before it is
   written, changed or removed (`time.period_locked`), and a locked record is corrected like an
   approved one.
5. The workspace chooses FULL or AGGREGATED visibility of other people's time; aggregated, another
   person's week is read only while it waits for a decision.
6. REST, MCP and hubctl list, read, approve and send back weeks and read and set the policy alike.
7. The web app shows an approver the queue of weeks to decide and lets them approve or send one back.

## Where it ends

* Approval runs in one step by one approver; approval chains are the workflows' (M5).
* Overtime and flexitime accounts are not built.
