---
id: UC-TIM-02
title: Fill in and hand in my week
context: timekeeping
actors: [PE-member, PE-integrator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-18]
state: built
tasks: []
checked_by: [core/domain/model/timekeeping/Timekeeping_test.go, core/application/service/timekeeping/Keeper_test.go, test/integration/time_repository_test.go, apps/webapp/src/lib/data/time.test.ts]
---

# Fill in and hand in my week

## Goal

A person sees their week from Monday as a grid of days, fills in what is missing, copies last
week's regular work, and hands the week in once it is complete.

## Story

On Friday Ben opens his week: Monday 8 hours, Wednesday empty. He copies last week, which brings
back the Wednesday workshop, adjusts Thursday and hands the week in - 40 hours.

## How to check

1. A week answers its state, its records and each day's minutes from Monday to Sunday, and which
   days lie in the locked period.
2. Copying the week before brings each of the person's records into the same weekday of an open
   week as a new record; corrections and locked days are left out.
3. A week is handed in with its total; an empty week is refused with `timesheets.empty`.
4. Handed in or approved, its records are neither written, changed nor removed, and a timer does
   not stop into it (`time.week_not_open`); taken back before a decision, it is open again.
5. A week sent back carries the approver's reason, and the person changes it and hands it in again.
6. A week stays in its workspace and is in the archive and in the person's access export.
7. REST, MCP and hubctl read, copy, hand in and take back a week alike.
8. The web app shows the week as a grid of days with the records on their entries, copies last
   week and hands it in.

## Where it ends

* Reminders for a week not handed in, and the hours a person is expected to work, are not built.
