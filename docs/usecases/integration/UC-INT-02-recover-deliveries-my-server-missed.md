---
id: UC-INT-02
title: Recover the deliveries my server missed
context: integration
actors: [PE-integrator, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-11]
state: built
tasks: [G-03, G-09, F4-15]
checked_by: [core/application/service/integration/Deliveries_test.go, core/application/service/integration/SendWebhook_test.go, test/integration/webhook_test.go]
---

# Recover the deliveries my server missed

## Goal

After an outage on the receiving side, the integrator sees exactly which events did not arrive and
sends them again — with the same identity, so the receiver can tell a repeat from a new event.

## Story

The administrator opens the subscription's deliveries and filters to *dead letter*. Twelve
deliveries failed while the receiver was misconfigured. After fixing it, they replay each one; the
receiver deduplicates on the event identifier and processes the ones it never saw.

## How to check

1. A subscription's deliveries are listed newest first, with the status, the response the target
   gave, the attempt count and when the next attempt is due; the list can be narrowed by status.
2. Only a dead-lettered delivery can be replayed; any other is refused with
   `webhooks.delivery_not_replayable`.
3. A replayed delivery carries the event identifier it always had, and its attempt counter carries
   on from where it stopped rather than starting at one.
4. A replay is in the trail as `webhooks.delivery_replayed`, naming who replayed it.
5. One named event can be sent to one named subscription on purpose — also as a rule action — even
   if the subscription does not list that event type; a paused or switched-off subscription is
   refused with `webhooks.subscription_not_active`.
6. An event a restore replayed is never sent this way.
7. The same list and replay are available through `hubctl webhook deliveries` and
   `hubctl webhook replay`.

## Where it ends

* Deleting a subscription deletes its delivery log with it.
* No bulk replay of every dead letter in one call.
* Events older than the outbox's retention are gone; recovering them is not possible.
