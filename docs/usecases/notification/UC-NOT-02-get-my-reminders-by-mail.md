---
id: UC-NOT-02
title: Get my reminders by mail
context: notification
actors: [PE-person, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-11, P-12]
state: built
tasks: [D-02, D-03, F3-13]
checked_by: [core/application/service/notification/RecordReminder_test.go, test/integration/reminder_firing_test.go]
---

# Get my reminders by mail

## Goal

A reminder a person set arrives by mail at the moment they asked for — once, to the people it
names — and never for something already done.

## Story

The person sets a reminder *one hour before* on an entry due at 15:00. At 14:00 a mail arrives:
*Reminder: "Call the plumber"*, with a link. On another entry they completed the work early; its
reminder never arrives, and the entry shows the reminder as cancelled.

## How to check

1. At the reminder's moment one mail goes to each person it names, or — if it names nobody — to
   the entry's assignee and members.
2. A reminder is sent at most once, also when a worker restarts in the middle of sending.
3. A reminder for an entry that is completed, in the trash or archived is not sent; it is marked
   cancelled, and the entry shows that.
4. A reminder reaches its recipient even if they set it themselves.
5. A person who switched reminder mails off receives none, and the reminder still counts as sent.
6. The mail carries the entry's title and a link, and no notes.
7. When the entry's due date moves, the reminder's moment moves with it.

## Where it ends

* Setting, changing and deleting a reminder are the work context's use cases; this is only the
  delivery.
* Mail only. A device may show its own local notification, but that is the device's, and it has
  to reconcile with the server's state ([offline-sync.md](../../architecture/offline-sync.md) §8).
* No snooze from the mail.
