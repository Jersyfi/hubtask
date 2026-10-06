---
id: UC-INS-06
title: Let a purchase platform provision workspaces with its own credential
context: admin
actors: [PE-operator, PE-platform]
deployments: [D5, D6]
serves: [P-08, P-15]
state: partial
tasks: [SI-05, SC-05]
checked_by: []
---

# Let a purchase platform provision workspaces with its own credential

## Goal

A provider connects its shop or customer platform to Hubtask with a credential that belongs to a
machine, not to an employee: the platform creates a workspace when somebody buys, suspends it when
they stop paying, resumes it when they pay — and it keeps working when the employee who set it up
leaves.

## Story

The operator creates a service account in the operator workspace, makes it an operator from the
*Operators* screen (a list of service accounts, not an address), and gives it a token with the
administrative scope. The platform calls `POST /admin/tenants` with an idempotency key on every
purchase; a webhook arriving twice creates one workspace.

## How to check

1. A service account can be made an operator from the *Operators* screen and from `hubctl`,
   choosing it by name.
2. Its administrative token can be created from the web app and from `hubctl`, with a fresh proof
   by the person creating it.
3. Provisioning with the same idempotency key twice creates one workspace and answers the same
   result both times.
4. Suspend and resume are idempotent and in the instance journal and in the workspace's own trail.
5. When the person who set it up is removed from the register, the platform's credential keeps
   working.

## Where it ends

* No prices, contracts or payment state in Hubtask ([NG-billing](../../vision/non-goals.md)).
* Telling the platform what happened is UC-INS-14; plans are UC-INS-13.

## Today

* Check 1: not met — the *Operators* screen adds operators only by address, which a service account does not have, tracked in #1061.
* Check 2: not met in `hubctl` — `hubctl token create` never sends a step-up, so an administrative token cannot be created from the terminal, tracked in #1061.
