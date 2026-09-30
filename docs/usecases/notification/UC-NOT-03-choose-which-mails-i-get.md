---
id: UC-NOT-03
title: Choose which mails I get
context: notification
actors: [PE-person, PE-member, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-06, P-12, P-13]
state: built
tasks: [C-09, F3-02, F3-17]
checked_by: [core/domain/model/notification/Preference_test.go, test/integration/notification_test.go, apps/webapp/src/lib/data/preferences.test.ts]
---

# Choose which mails I get

## Goal

A person decides, per kind of message, whether they are mailed at all and whether the mail may
name the entry — in one place, with everything on until they say otherwise, and with the one
message that lets them in never switchable off.

## Story

Under *Profile → Notifications* the person sees one row per kind — assignments, being added,
comments, invitations, reminders, integrations, retention — each with *Send* and *Include the
title*. They switch comments off and switch the title off for everything else, because their mail
is read on a shared screen. From then on comment mails stop, and the others say only that something
concerns them, with a link.

## How to check

1. Every kind of message is listed with its two switches, and both are on for somebody who has
   never changed anything.
2. Switching a kind off stops the next mail of that kind; the record of it says it was suppressed
   because the kind is off.
3. Switching *Include the title* off produces a mail that names no entry and no rule, only that
   something concerns the reader, with a link.
4. The invitation cannot be switched off; the screen shows it as always on and says why.
5. A person changes their own preferences without any permission; changing somebody else's needs
   the permission that manages members.
6. Each change is in the trail.
7. An unknown kind or channel is refused with `notifications.category_unknown` or
   `notifications.channel_unknown`.

## Where it ends

* No quiet hours, no digest, no per-collection or per-entry switch.
* No workspace-wide defaults set by an administrator; the default is on.
* Password-reset mails are not a preference; they are part of signing in.
