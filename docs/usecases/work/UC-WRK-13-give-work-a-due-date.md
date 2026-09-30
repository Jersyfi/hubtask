---
id: UC-WRK-13
title: Give work a due date
context: work
actors: [PE-person, PE-member, PE-child, PE-agent, PE-scripter, PE-integrator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-08, P-11, P-12, P-13]
state: built
tasks: [D-01, F3-06, F3-12, F10-14, M-06]
checked_by: [test/integration/due_date_test.go, core/domain/model/work/DueDate_test.go, core/application/service/work/SetDueDate_test.go, apps/webapp/e2e/timeline.test.mjs]
---

# Give work a due date

## Goal

A person says when something is due — a whole day ("Friday") or a moment ("Friday 14:00") — and,
where it matters, when it starts; the date means the same thing wherever and whenever it is read,
and overdue work looks overdue.

## Story

The person opens "Tax return", picks 31 July and leaves *all day* on. The list shows the date in
their own language and calendar week; on 1 August the row turns overdue. A meeting prep gets
Friday 14:00 and a start date on Wednesday, and the timeline draws a bar from Wednesday to Friday.
A colleague in Lisbon sees 13:00 and a note that it was set in Berlin time.

## How to check

1. The due control offers a date, a time, *all day* and *clear*; a start date and time sit beside
   it.
2. An all-day date stays that calendar day for every reader in every time zone; a date with a time
   is one moment, shown in each reader's own time zone.
3. An entry whose date was set in another time zone than the reader's says so and shows both
   times.
4. An open entry past its due day (all day) or its due moment (with a time) is marked overdue in the
   list, on the board, on the entry and in the overview; a completed one is not.
5. Dates are written in the reader's language and format and weeks start on the reader's chosen
   first day.
6. Setting or moving the date writes `item.due_set` with both sides in the history; clearing it
   writes `item.due_cleared`.
7. On the timeline, dragging a bar moves both dates, dragging an end moves one, and dragging an
   undated entry out of the tray gives it its first dates.
8. A malformed date through the API is refused with `items.due_unreadable` or
   `items.due_at_malformed`; a time zone without a date with `items.due_time_zone_without_date`.

## Where it ends

* No choosing a different time zone for one entry in the web app — the reader's own zone is used;
  the API accepts one for a script that needs it.
* No working-day calendars, holidays or durations in work days.
* Being reminded is [UC-WRK-14](./UC-WRK-14-be-reminded-before-something-is-due.md); a calendar
  subscription to the dates belongs to the integration context.
