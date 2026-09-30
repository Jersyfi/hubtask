---
id: UC-WRK-14
title: Be reminded before something is due
context: work
actors: [PE-person, PE-member, PE-child, PE-selfhoster, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-10, P-11, P-12]
state: partial
tasks: [D-02, D-03, F3-13]
checked_by: [test/integration/reminder_test.go, test/integration/reminder_firing_test.go, core/domain/model/work/Reminder_test.go, core/application/service/work/FireReminders_test.go]
---

# Be reminded before something is due

## Goal

A person who set a date is told in time — a day before, an hour before, at a moment they chose —
and the people on the entry are told with them. If the installation cannot deliver a reminder,
the person setting it learns that then, not after the deadline.

## Story

On "Dentist" the person adds *1 hour before* and *1 day before*. Tomorrow they receive a mail with
the entry's current title; if it was renamed in between, the mail says the new title. A reminder
with no recipients reaches whoever is responsible for the entry and whoever is on it *when it
fires*, so somebody added later is reminded too. In a household that never set up a mail server,
the reminder panel says reminders cannot reach anybody until mail is configured.

## How to check

1. The reminder panel offers presets — at the due time, 10 minutes, 1 hour, 1 day, 1 week before —
   and a custom amount before the due time or a fixed date and time.
2. A reminder relative to the due date on an entry without one is refused with a sentence saying
   a due date is needed; a fixed-time reminder needs none.
3. Moving the due date moves every relative reminder with it; a reminder whose moment has passed
   fires once, not again.
4. With no recipients chosen, the reminder reaches the responsible person and the entry's members
   as they are when it fires; chosen recipients must be able to see the entry.
5. A fired reminder carries the entry's title as it is at that moment, respects each recipient's
   notification preference for reminders, and is shown as fired rather than pending.
6. An entry holds at most the published number of reminders (25 by default); the panel stops
   offering more at that number.
7. On an installation **without a mail server**, the reminder panel says that reminders cannot be
   delivered yet, before the person relies on one.
8. The panel lists each reminder in words ("1 hour before", "on 3 May at 09:00"), not as its
   stored form.

## Where it ends

* Mail is the one channel; push and in-app reminders belong to the notification context and are
  not built.
* No snoozing from the mail.
* Reminders are copied neither by a duplicate nor by a template.
* The operator's side — the startup warning that reminders wait for a mail server — is the
  installation's, not this use case's.

## Today

* **Check 7 fails.** The manifest always publishes `EMAIL` as a channel
  (`core/application/service/meta/GetCapabilities.go:474`) and the panel offers it whether or not a
  mail server is configured; only the server's log says `config.smtp_missing_with_reminders`. In
  `D1` and `D2` without mail, a reminder is stored and silently never reaches anybody.
* **Check 8 fails.** The list shows the stored form, for example `REL:-PT1H`
  (`apps/webapp/src/lib/entries/ReminderPanel.svelte:172`).
