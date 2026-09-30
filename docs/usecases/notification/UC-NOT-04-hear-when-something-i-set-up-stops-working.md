---
id: UC-NOT-04
title: Hear when something I set up stops working
context: notification
actors: [PE-admin, PE-owner, PE-integrator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-11, P-12]
state: built
tasks: [G-03, G-07, F8-03]
checked_by: [core/application/service/notification/RecordRuleDisabled_test.go, test/integration/notification_rule_test.go]
---

# Hear when something I set up stops working

## Goal

Whoever set up a rule or a webhook subscription learns by mail, the moment the system gives up on
it, that it has stopped — so it is not discovered weeks later by its absence.

## Story

A rule's destination collection was deleted; after five failures in a row the rule switches itself
off, and its author receives *Hubtask switched off the rule "Route invoices"* with a link to the
rule. A webhook receiver has been gone for days; after three dead-lettered deliveries the
subscription is switched off, and the person who created it receives *Hubtask stopped calling
"n8n tasks"*.

## How to check

1. A rule switched off after five failed runs, or by the check as broken, mails its author once,
   naming the rule and linking to its page.
2. A webhook subscription switched off as unreachable mails the person who created it once, naming
   the subscription.
3. The mail goes to the author, never to the account the rule runs as: a service account has
   nobody to read it.
4. These mails are the *integration* kind: switching comments or assignments off does not silence
   them, switching the integration kind off does.
5. With *Include the title* off, the mail says a rule or subscription was switched off without
   naming it.
6. An author whose account is gone is not mailed, and the record says why.

## Where it ends

* No mail for a single failed run or a single failed delivery; only for the conclusion.
* No mail to the whole administration team; the author is the one who is told.
