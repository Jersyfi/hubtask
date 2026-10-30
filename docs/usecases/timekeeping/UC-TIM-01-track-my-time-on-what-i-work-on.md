---
id: UC-TIM-01
title: Track my time on what I work on
context: timekeeping
actors: [PE-member, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-07, P-18]
state: built
tasks: []
checked_by: [core/domain/model/timekeeping/Timekeeping_test.go, core/application/service/timekeeping/Keeper_test.go, core/application/service/timekeeping/Timers_test.go, core/application/service/sync/Entity_test.go, apps/webapp/src/lib/data/replica.test.ts, test/integration/time_repository_test.go, apps/webapp/src/lib/data/time.test.ts]
---

# Track my time on what I work on

## Goal

A person records the time they spend where the work is - on an entry, or on nothing in particular -
by hand or with one timer that every device shows, without a second tool beside the task manager.

## Story

Ana starts the timer on "Checkout form" at nine. At ten to ten she stops it on her phone: 51
minutes of development, billable, on Monday. Her stand-up she records by hand as 15 minutes of
meeting on no entry.

## How to check

1. A person records minutes from 1 to 1440 on one day, or a start and an end the minutes are
   counted from (rounded up to the started minute), with a note of at most 2000 characters; more or
   fewer minutes are refused with `time.minutes_out_of_range`.
2. The time lies on an entry the person may read or on none, of one of the workspace's kinds of
   work, billable as that kind says unless the person says otherwise; an entry out of reach answers
   not found, an unknown or archived kind `work_types.not_found`.
3. One timer runs per person (`timers.running`); stopped, it is a record of the day it started
   in the person's zone, and a timer run longer than a day is refused with `timers.ran_too_long`
   until it is discarded.
4. A person changes and removes only their own records; another person's record answers not found.
5. The note never reaches the audit trail; the day, the minutes, the kind, the entry and the
   billable flag do.
6. A record stays in its workspace and is in the workspace's archive and in the person's access
   export.
7. REST, MCP and hubctl record, change, remove and list time and start, stop, read and discard the
   timer alike.
8. The web app shows the running timer on an entry and lets a person start and stop it there.
9. While the server is away a device starts, stops and discards the timer and shows it running;
   once the server is back, the record a stop wrote holds the minutes the timer ran on the device,
   a start more than a day old is refused with `timers.ran_too_long`, and every device shows the
   one timer.

## Where it ends

* No attendance clocking: working-time records under labour law are a different purpose.
* Offline, only the timer is written; records are recorded, changed and removed with the server.
