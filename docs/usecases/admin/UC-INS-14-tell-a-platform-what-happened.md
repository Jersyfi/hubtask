---
id: UC-INS-14
title: Tell the purchase platform what happened, without it asking
context: admin
actors: [PE-platform, PE-operator]
deployments: [D5, D6, D7]
serves: [P-08, P-11, P-15]
state: partial
tasks: [PL-07, PL-08]
checked_by: [test/contract/installation_events_test.go, core/application/service/platform/Recorder_test.go, core/application/service/admin/PlatformEvents_test.go, core/application/service/platform/Crossings_test.go, core/application/service/identity/OwnerChanges_test.go, core/application/service/quota/Crossings_test.go, core/application/service/admin/ExportFinished_test.go, test/integration/platform_event_test.go, core/application/service/admin/PlatformSubscriptions_test.go, infrastructure/webhook/PlatformDeliverer_test.go, test/integration/platform_subscription_test.go, presentation/worker/Scheduler_test.go, cmd/hubctl/AdminSubscription_test.go, cmd/hubctl/AdminEvents_test.go, infrastructure/instancefile/File_test.go, apps/webapp/e2e/instanceplatform.test.mjs]
---

# Tell the purchase platform what happened, without it asking

## Goal

A provider's platform learns what happens in Hubtask that matters for running the business — a
workspace created, moved to another plan, suspended, deleted, an owner changed, a limit reached,
the AI budget spent, an export finished — as signed events, and reads each workspace's use per
period for billing.

## Story

The operator subscribes the platform's address to installation events. Each event arrives signed,
in the same format a workspace's webhooks use, and is retried until delivered. Once a month the
platform reads the use per workspace and metric for the period.

## How to check

1. Installation-level webhook subscriptions exist, with the same signing, retries and dead-letter
   handling as a workspace's webhooks.
2. The events listed in the goal are delivered, each at most a few seconds after the change. Every
   event carries the workspace's whole current state — status, plan, the platform's reference, a
   version that only grows — so a receiver keeps the newest and needs no ordering; "a limit
   reached" is sent once when a limit is crossed, not for every refused request.
3. Events carry identifiers and states, never content.
4. Use per workspace, per metric and per day over any range can be read in one request, each
   metric saying which day it counts by (UTC or the workspace's zone) and the answer saying up to
   which day it is final; people and storage are recorded as each day's highest value.
5. A subscription and its deliveries are visible and can be retried from the installation level.
6. Besides the webhooks, the platform can read the installation's events forward from a cursor —
   the same events — so a platform that was down misses nothing; an installation subscription is
   never switched off silently: its dead letters stay re-deliverable and the operator is told.
7. `hubctl` and the instance file reach subscriptions and the use; single mode offers none of it.
8. Use is offered for billing only while the operator has switched metering on, off by default;
   after a workspace's purge its daily counts stay readable as bare numbers — no name, no
   reference — for 13 months, then go.

## Where it ends

* No invoice, no price, no dunning in Hubtask ([NG-billing](../../vision/non-goals.md)).
* The platform decides what to do with the events; Hubtask never waits for an answer.

## Today

* Check 4: not met — use cannot be read at all, and people and storage are not recorded (PL-08).
* Check 7: not met — `hubctl` and the file reach the subscriptions; the use is not readable (PL-08).
* Check 8: not met — use is always recorded, never offered, and goes with the workspace (PL-08).
