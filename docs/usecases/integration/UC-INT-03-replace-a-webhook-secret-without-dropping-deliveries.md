---
id: UC-INT-03
title: Replace a webhook secret without dropping deliveries
context: integration
actors: [PE-integrator, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-11]
state: built
tasks: [G-03, F4-15]
checked_by: [core/application/service/integration/Deliveries_test.go, core/domain/model/integration/WebhookSubscription_test.go]
---

# Replace a webhook secret without dropping deliveries

## Goal

A signing secret can be replaced on a schedule, with a stated overlap during which the receiver
still honours the old one, so it can be redeployed without losing an event — or replaced
immediately, when it has leaked.

## Story

The administrator rotates the secret with a grace of one day. The new secret is shown once, with
the moment the old one stops counting. They deploy the new secret to the receiver during the
afternoon; the receiver accepts either secret until the stated moment. A delivery the receiver
refused while it only knew the old secret is retried and arrives once it knows the new one. After a
leak they rotate with no grace, and the old secret counts for nothing from that moment.

## How to check

1. Rotating answers the new secret once, with the moment the previous secret stops counting; no
   read shows the secret again.
2. Every delivery after the rotation is signed with the new secret.
3. The previous secret counts until the grace given ends; without a grace, for one day; with zero,
   not at all.
4. A grace longer than seven days is refused with `webhooks.rotation_grace_too_long`, naming the
   maximum.
5. A delivery the receiver refuses during the switch is retried on the ordinary ladder, not
   dropped.
6. The rotation is in the trail as `webhooks.secret_rotated`, with no secret in it.
7. Rotating needs the automation permission on the workspace.

## Where it ends

* The overlap is the receiver's to honour: deliveries are never signed twice.
* No secret chosen by the integrator; the installation draws it.
