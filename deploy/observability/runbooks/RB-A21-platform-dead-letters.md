<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A21 — Events wait to be redelivered to the purchase platform

**Alert:** `HubtaskPlatformDeadLetters` · **Severity:** ticket · **Catalogue:** A-21

## The symptom

Deliveries of the installation's events to a purchase platform's subscription ended as dead
letters: eight attempts over a little more than two days did not reach it, or the subscription
was paused while events were on their way. Unlike a workspace's webhook, an installation
subscription is never switched off for this (multi-tenancy.md §4.3) — the events wait, and this
ticket is how the operator learns of them.

Nothing is lost while they wait: the platform can read every event from `GET /admin/events` with
the cursor it last kept, for 30 days.

## Immediate action

```bash
# Which subscriptions hold dead letters, and what did the target answer?
hubctl admin subscription ls
hubctl admin subscription deliveries <subscription-id> --status DEAD_LETTER
```

* **`webhooks.target_unavailable` / `webhooks.target_unreachable`** → the platform is down or its
  address moved. Once it answers again: `hubctl admin subscription redeliver <subscription-id>`,
  or *Installation → Ereignisse an die Plattform → Erneut zustellen*.
* **`webhooks.target_refused`** → the platform answers 4xx: a rotated secret it does not know yet,
  or an endpoint that changed. Fix it on the platform's side, then redeliver.
* **`webhooks.target_blocked`** → the guarded client refuses the address (a private range); the
  installation has not released private networks. Change the target or the release.
* **`admin.subscription_paused`** → events queued before a pause. Resume, then redeliver.
* **`webhooks.event_expired`** → the event was swept before it could be sent. Such a delivery is
  neither counted nor redelivered - nothing can be sent for it - and it goes with the sweep; the
  platform reconciles by reading the workspaces.

## Diagnostic queries

```promql
max(hubtask_platform_dead_letters)
sum by (result, status_class) (rate(hubtask_platform_deliveries_total[1h]))
```

## Escalation

Dead letters for one subscription only are the platform's matter. Dead letters across every
subscription with `webhooks.target_blocked` after an upgrade point at the outbound guard's
configuration — treat that as an incident of this installation.

## Follow-up

After a redelivery the gauge falls on the scheduler's next tick and the alert resolves. Dead
letters older than 30 days are swept; the platform reconciles those through the feed or by
reading the workspaces.
