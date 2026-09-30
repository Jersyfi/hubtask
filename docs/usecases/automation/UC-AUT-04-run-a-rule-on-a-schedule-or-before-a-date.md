---
id: UC-AUT-04
title: Run a rule on a schedule, or a set time before or after a date
context: automation
actors: [PE-admin, PE-owner, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-10, P-11]
state: built
tasks: [G-08]
checked_by: [core/application/service/automation/SchedulePass_test.go, core/application/service/automation/RelativeDates_test.go, test/integration/automation_trigger_test.go]
---

# Run a rule on a schedule, or a set time before or after a date

## Goal

A rule can act at a time rather than on an event — every Monday at eight, or a day before each
entry's due date — at the moment the person meant, in their time zone, through daylight saving
changes, and without a flood after an outage.

## Story

The administrator writes a rule that runs *weekly on Monday at 08:00, Europe/Berlin* and creates the
weekly review task. Another rule runs *24 hours before the due date* of every entry in a collection
and adds a label. When a due date moves, the moment moves with it; when an entry is completed or
its due date cleared, the rule owes it nothing.

## How to check

1. A scheduled rule runs at each moment its recurrence names, in the time zone it names, including
   on the days daylight saving time starts and ends.
2. A schedule without a time zone is refused with `automation.timezone_required`, an unknown zone
   with `automation.timezone_unknown`, a recurrence the installation cannot read with
   `automation.rrule_unreadable` — all at the save, not at the first run.
3. Switching a scheduled rule on counts from now: a schedule that was off for a week owes nothing
   for that week.
4. After the workers were down across several due moments, the rule runs once to catch up and
   then continues forward — not once per missed moment.
5. A rule relative to a date runs at the offset from each matching entry's date, and when that date
   changes, the run moves with it.
6. An entry whose date is cleared, that is in the trash or that is gone gets no run.
7. A rule relative to a date does not walk the entries that existed before it was switched on: an
   entry is owed a run once its date is set or changed after that.
8. One workspace's schedules never cause runs in another; a workspace that owes nothing costs
   nothing until a write makes it owe something.

## Where it ends

* No cron syntax; recurrences are written the way recurring tasks are.
* No per-person time zone for a rule: the rule carries its own.
* Recurring tasks themselves are not this use case; they belong to the entry and need no rule.
