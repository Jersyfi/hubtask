---
id: UC-INT-16
title: Write my calendar feed into my Outlook or Google calendar
context: integration
actors: [PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-08, P-09, P-12]
state: built
tasks: []
checked_by: [core/domain/model/integration/CalendarPush_test.go, core/application/service/calendarpush/CalendarPush_test.go, infrastructure/calendarpush/Calendar_test.go, test/integration/calendar_push_test.go, apps/webapp/src/lib/data/calendarpush.test.ts]
---

# Write my calendar feed into my Outlook or Google calendar

## Goal

A person who lives in Outlook or Google Calendar sees their due work there within a quarter hour,
in their own calendar rather than a subscription the provider refreshes when it pleases, without
the workspace learning anything about the rest of that calendar.

## Story

Ada has a calendar feed of her view *Due this week* (UC-INT-06). In the view's calendar
subscriptions she chooses *Write into Outlook*, signs in at Microsoft and allows Hubtask the
calendar's events - nothing else. Within minutes her due entries stand in her Outlook calendar as
free events without a reminder, each with its title and a link back. When an entry moves, its
event moves; when it leaves the view, its event goes. Under *Your settings → Calendars* she sees
that it writes and when it last did, and disconnects it there.

## How to check

1. The owner connects their own calendar feed to Microsoft 365 or Google through the provider's
   consent with the installation's app, asking for the calendar's events, an offline grant and who
   signed in - never mail or files; the push keeps the account signed in with and its grant
   sealed. Somebody else's feed, a data feed or a revoked one is not found, a provider without an
   app on the installation is not offered, and a sign-in state is good once, for ten minutes, for
   whoever started it.
2. A run reads the feed as its owner reads it now and writes each dated entry as an event with its
   title, its moment (a timed entry for half an hour, an all-day entry on its day) and a link
   back, shown as free and without a reminder; an entry that changed is updated in place, one the
   feed no longer serves is removed, an unchanged one is not written again, and an event its owner
   removed at the provider is written anew.
3. Runs repeat every quarter hour; *Your settings → Calendars* lists the reader's own pushes only,
   with the provider, the account, whether it writes and when it last did.
4. A grant the provider no longer takes, or a feed that is gone, stops the push and says why; a
   provider that cannot be reached is tried again at the next run. Disconnecting forgets the grant
   and writes nothing more.

## Where it ends

* The events written stay in the calendar when the push is disconnected; their owner removes them
  there.
* One calendar per provider account, its default one; an entry's notes, people and labels never
  travel, as in the feed (data-protection.md §9).
* A push is the person's own credential: it is not in the workspace archive nor restored, and is
  connected again after a move.
* Nothing is read back from the calendar: an event moved there does not move its entry.
