---
id: UC-NOT-06
title: Be told somewhere other than my mailbox
context: notification
actors: [PE-person, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-06, P-10, P-12]
state: specified
tasks: []
checked_by: []
---

# Be told somewhere other than my mailbox

## Goal

A person who does not live in their mailbox — or a household without a mail server — receives the
same notifications on a channel they choose, under the same preferences, and each kind of message
can go to a different channel.

## Story

The person opens *Profile → Notifications* and finds a column per channel beside *Email*. They
send reminders to their phone and comments nowhere, and keep assignments by mail. A reminder now
appears as a notification on the phone at its moment; nothing arrives by mail for it.

## How to check

1. The notification preferences offer each available channel per kind of message, with the same
   two switches as mail.
2. A message is delivered on every channel its kind is switched on for, and on no other.
3. A channel the installation does not offer is not shown.
4. The same rules as for mail hold on every channel: nobody is told about their own act, only
   people on the entry are told, and the message carries the title and a link at most.

## Where it ends

* Which channels are built — push to the installed apps, a personal webhook, something else — is a
  decision this use case does not make; [arc42](../../architecture/arc42.md) names a webhook and a
  push gateway.
* No SMS ([NG-sms](../../vision/non-goals.md)).
* No third-party push service the operator did not configure
  ([NG-phone-home](../../vision/non-goals.md)).

## Today

* Check 1: not met — mail is the only channel in the domain, so the preferences offer no other.
* Check 2: not met — there is no channel other than mail to deliver on.
* Check 3: not met — the preferences show the channels the manifest publishes, and it always publishes mail, also on an installation without a mail server.
* Check 4: not met — there is no channel other than mail on which the rules could hold.
