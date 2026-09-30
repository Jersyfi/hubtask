---
id: UC-INT-01
title: Receive the workspace's events on my own server
context: integration
actors: [PE-integrator, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-08, P-09, P-11]
state: partial
tasks: [G-02, G-03, F4-15]
checked_by: [core/application/service/integration/Webhook_test.go, core/application/service/integration/SendWebhook_test.go, test/integration/webhook_test.go, test/security/ssrf_test.go]
---

# Receive the workspace's events on my own server

## Goal

An outside system learns within moments that something happened in the workspace — an entry
created, completed, moved — as a signed message it can trust and deduplicate, and keeps receiving
them reliably even when it is briefly down.

## Story

In *Administration → Webhooks* the administrator subscribes `https://n8n.example.org/hook/tasks` to
*entry created* and *entry completed*. The signing secret is shown once; they paste it into n8n.
From then on each such event is posted there as a CloudEvent, signed. When n8n is down for an hour
the deliveries are retried; when it has been unreachable for good, the subscription switches itself
off and the administrator is told.

## How to check

1. Subscribing needs the automation permission on the workspace; the answer carries the signing
   secret once, and no listing or read shows it again.
2. Only event types the installation declares can be subscribed to; an unknown one is refused with
   `webhooks.event_type_unknown`, naming it.
3. A target in a private or link-local network, or the cloud metadata address, is refused unless
   the installation's operator released private networks.
4. Each delivery is a CloudEvents 1.0 document, identical to the event inside the product, with an
   `X-Hubtask-Signature` of timestamp and HMAC-SHA256 over timestamp and body, and the headers
   `X-Hubtask-Event-Id`, `X-Hubtask-Event-Type` and `X-Hubtask-Delivery-Attempt`.
5. A failed delivery is retried up to 8 attempts with growing delays over up to a day, then ends as
   dead-lettered.
6. After three dead-lettered deliveries in a row the subscription is switched off as unreachable,
   the trail holds `webhooks.subscription_disabled`, and its creator is told by mail unless they
   switched integration messages off.
7. A subscription can be paused and resumed by hand; being switched off as unreachable is not a
   state anybody can set, and resuming one is an ordinary audited change.
8. Many changes a device pushes to one entry in one synchronisation arrive as one delivery per
   event type, not one per change; an event a restore replayed is never delivered.
9. A subscription can be narrowed by a condition over the event, and to one hub or collection,
   rather than receiving every event of its types in the workspace.

## Where it ends

* No custom payload shape: the CloudEvent is the one schema, the same one polling answers.
* No ordering guarantee across events; the event identifier is what a receiver deduplicates on.
* Delivering one event on purpose, recovering dead letters and rotating the secret are the next two
  use cases.

## Today

* **Check 9 fails.** A condition (`filter`) is refused with `webhooks.filter_not_supported`
  (`core/domain/model/integration/WebhookSubscription.go:215`), and a subscription takes no scope
  at all — both promised in [automation.md](../../architecture/automation.md) §3.1. Every
  subscription receives every event of its types in the whole workspace.
