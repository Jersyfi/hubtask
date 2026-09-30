---
id: UC-INS-13
title: Put workspaces on plans with their own limits, features and locks
context: admin
actors: [PE-operator, PE-platform, PE-owner]
deployments: [D5, D6]
serves: [P-03, P-05, P-07, P-15]
state: specified
tasks: []
checked_by: []
---

# Put workspaces on plans with their own limits, features and locks

## Goal

A provider groups workspaces into plans — "Private", "Family", "Business" — each with its own
limits, its own switched-on features and its own locked values, so that consumers and companies can
live on one installation; changing a plan takes effect at once and never destroys anything.

## Story

The operator defines plans on the installation. The platform provisions a workspace on "Family"
and moves it to "Business" when the customer upgrades; the new limits and features apply at the
next request. A workspace that loses a feature by downgrading keeps its data and configuration —
the rules stop running, the second provider stops signing people in — and gets both back on
upgrading.

## How to check

1. A plan carries values with locks, limits and feature switches; a workspace references its plan
   and nothing is copied into the workspace.
2. Changing a plan's value changes it for every workspace on the plan at the next request, without
   a job that visits workspaces.
3. A workspace's screens say "Set by your plan" where the plan locked a value, and "Not included in
   your plan" where a feature is off — without selling anything.
4. Downgrading refuses new items beyond a limit and stops removed features; nothing existing is
   deleted, and a value the plan newly locks is kept and applies again when the lock goes.
5. Export, deletion, access requests, language, time zone and accessibility can never be switched
   off by a plan.
6. A plan change is in the instance journal and in the workspace's own trail.

## Where it ends

* No prices, trials or invoices ([NG-billing](../../vision/non-goals.md)).
* No per-seat licensing inside Hubtask; a limit on accounts is a limit like any other.

## Today

* **Not built.** Prepared by SI: `tenant.plan_id`, the resolver's plan parameter and the lock's
  origin. Its own milestone, after passkeys.
