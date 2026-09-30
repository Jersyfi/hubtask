---
id: UC-INS-14
title: Tell the purchase platform what happened, without it asking
context: admin
actors: [PE-platform, PE-operator]
deployments: [D5, D6]
serves: [P-08, P-11, P-15]
state: specified
tasks: []
checked_by: []
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
2. The events listed in the goal are delivered, each at most a few seconds after the change.
3. Events carry identifiers and states, never content.
4. Use per workspace, per metric and per period can be read in one request.
5. A subscription and its deliveries are visible and can be retried from the installation level.

## Where it ends

* No invoice, no price, no dunning in Hubtask ([NG-billing](../../vision/non-goals.md)).
* The platform decides what to do with the events; Hubtask never waits for an answer.

## Today

* **Not built.** Named by the concept's §6.2; belongs to the plans milestone.
