---
id: UC-INT-07
title: Tick off and reschedule tasks from my calendar app
context: integration
actors: [PE-person, PE-member, PE-integrator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-05, P-08, P-09]
state: built
tasks: [P-06, P-07, F6-11]
checked_by: [presentation/calendar/CalDav_test.go, presentation/calendar/CalDavWrite_test.go]
---

# Tick off and reschedule tasks from my calendar app

## Goal

A person whose day runs in a calendar or reminders app sees their entries there as to-dos and can
tick them off, move their date, rename them or add a new one — and each change is the same change
as a click in Hubtask, with the same rights and the same history.

## Story

The person makes a personal access token and adds a CalDAV account to their phone with the
installation's address, their user name and the token as the password; the web app shows the
address to use beside their calendar subscriptions. Each subscription appears as a to-do list. They
tick an item done on the phone; in Hubtask the entry is completed, with the history saying so. They
drag another to Friday; its due date moves.

## How to check

1. A CalDAV client given only the host finds the calendars through `/.well-known/caldav`; each of
   the person's calendar subscriptions is one to-do calendar, and nobody else's appear.
2. The client signs in with the account and a personal access token as the password; a revoked or
   expired token is refused.
3. Ticking a to-do complete completes the entry; unticking reopens it; changing its date sets or
   clears the due date; renaming it changes the title; deleting it moves the entry to the trash.
   A new to-do creates a task in the view's collection; in a calendar whose view names no single
   collection, a new to-do is refused (403) rather than placed somewhere the person did not
   choose.
4. Each change runs through the same use case as in the app: the same permission check, the same
   trail entry, the same history step. A change the person may not make is refused.
5. Changing an existing to-do without the version the client last read is refused (428); a change
   based on an outdated version is refused (412) rather than overwriting a newer one.
6. A property Hubtask does not keep — a description, a priority, categories — is refused by name
   instead of being silently dropped.
7. The same to-do keeps the same identity across fetches, so the client updates rather than
   duplicates.

## Where it ends

* No incremental sync token; clients learn about changes by asking what changed and re-reading
  the list.
* No events, only to-dos. No sharing a calendar with somebody else; each person subscribes their
  own.
* No sign-in with the account password; only a token, which can be revoked on its own.
