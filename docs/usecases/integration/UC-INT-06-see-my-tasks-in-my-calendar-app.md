---
id: UC-INT-06
title: See my tasks in my calendar app
context: integration
actors: [PE-person, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-09, P-11, P-12]
state: built
tasks: [D-08, F3-15]
checked_by: [test/integration/calendar_feed_test.go, test/integration/calendar_acceptance_test.go, test/security/calendar_feed_test.go, presentation/calendar/Ics_test.go]
---

# See my tasks in my calendar app

## Goal

A person sees the entries with due dates from a view of theirs in the calendar they already use on
every device — read-only, current, and revocable at once if the address gets out.

## Story

On a collection the person opens *Calendar subscriptions*, picks the saved view *My deadlines*, and
makes a subscription. The address is shown once, with the warning that whoever has it reads the
view as they do. They paste it into their calendar app. Their due entries appear there and update
when dates change. When they change phones and no longer trust the old one, they revoke the
subscription; the calendar stops receiving anything.

## How to check

1. A subscription is made over one saved view; one without a view is refused with
   `calendar.view_required`.
2. The address is shown once, when it is made; the list of subscriptions shows each one's view and
   when it was made, never the address.
3. Fetching the address answers an iCalendar document with one item per entry of the view that has
   a due date: its title, its due moment (as a date for all-day entries), a link back and when it
   last changed — no notes, no assignee, no comments.
4. The document shows exactly what its owner may read today: an entry the owner lost access to
   disappears from the next fetch.
5. Revoking a subscription makes its address answer not-found from the next fetch on; an unknown,
   malformed or revoked address all answer the same.
6. The subscriptions listed are only the person's own, whatever their role; an administrator
   cannot list or read another person's.
7. The document names no version of the installation.

## Where it ends

* Read-only: changes in the calendar are not written back through this address. Writing back is
  the CalDAV use case.
* No free/busy, no events that are not entries, no invitations.
* A subscription does not travel to devices and is not made offline.
