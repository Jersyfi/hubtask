---
id: UC-WRK-15
title: Repeat a task
context: work
actors: [PE-person, PE-member, PE-child, PE-agent, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-08, P-11, P-12]
state: partial
tasks: [D-04, D-05, F3-13]
checked_by: [test/integration/recurrence_test.go, test/integration/materialisation_test.go, core/domain/model/work/Recurrence_test.go, core/application/service/work/Recurrence_test.go, core/application/service/work/MaterializeOccurrences_test.go]
---

# Repeat a task

## Goal

Work that comes back — the bins every Tuesday, rent on the first, the filter every three months
after it was last changed — is written once and turns up again by itself, at the same local time
all year round, and each time as its own entry that can be done, moved or skipped.

## Story

The person gives "Put the bins out" a due date of Tuesday 19:00 and makes it repeat *weekly on
Tuesday*. The coming weeks appear as their own entries, each due Tuesday 19:00 local time — also
after the clocks change. "Change the water filter" repeats *three months after it was done*: the
next one appears when the current one is completed. A holiday week is skipped with *Skip next*.
Stopping the series keeps the entries already made.

## How to check

1. The recurrence panel builds daily, weekly (with weekdays), monthly (on a day of the month) and
   yearly rules with an interval, and accepts an RFC 5545 rule typed by hand; it ends never, on a
   date, or after a number of times.
2. A series needs the entry's due date to count from; without one it is refused with
   `recurrence.due_date_required`.
3. *On schedule* creates each occurrence ahead of time, as far ahead as the horizon (90 days by
   default, 1 to 365); *after completion* creates the next one only when the current one is
   completed.
4. Each occurrence is its own entry, with the task's subtree, linked to the entry it repeats from;
   completing one completes nothing else.
5. An occurrence keeps its local wall-clock time across a daylight saving change in the series'
   time zone.
6. A rule that would produce more than 500 occurrences within its horizon is refused with
   `recurrence.rrule_too_dense`; a rule with both an end date and a count with
   `recurrence.end_spec_ambiguous`.
7. *Skip next* skips exactly one occurrence per press; removing the series asks first and leaves
   the entries already created untouched.
8. The panel states the series in words ("every week on Tuesday, until 31 December"), not as the
   rule's text.
9. The history records `item.recurrence_set`, `item.recurrence_changed` and
   `item.recurrence_removed` as three different sentences.

## Where it ends

* Only a task repeats; a work package or an activity repeats as part of its task's subtree.
* One rule per series — no exception dates, no second rule
  (`recurrence.rrule_not_single`).
* No choosing the series' time zone in the web app: it is the due date's, else the reader's.
* Editing "this and all following" occurrences is not offered; a changed rule applies to
  occurrences not yet created.

## Today

* **Check 8 fails.** When not being edited, the panel shows the stored rule text, for example
  `FREQ=WEEKLY;BYDAY=TU` (`apps/webapp/src/lib/entries/RecurrencePanel.svelte:192`).
