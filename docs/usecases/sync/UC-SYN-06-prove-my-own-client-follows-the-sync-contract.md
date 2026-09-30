---
id: UC-SYN-06
title: Prove that my own client follows the sync contract
context: sync
actors: [PE-integrator, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-08, P-09, P-11]
state: built
tasks: [N-12, N-13, F6-08]
checked_by: [cmd/hubctl/Conformance_test.go, cmd/hubctl/Sync_test.go, packages/sync-engine/test/conformance.test.ts]
---

# Prove that my own client follows the sync contract

## Goal

Somebody who writes their own offline client — a terminal tool, a native app — can check it against
a running installation and get a plain report of which of the contract's requirements it meets,
instead of learning about a data-losing mistake from a user.

## Story

The integrator runs `hubctl sync-conformance` against their test installation. It acts as two
devices and reports one row per requirement: identifiers, repeated pushes, lost access, a position
that is too old, refused changes, fields it does not know, and kinds that need the connection.
Their own client uses the same pull and push that `hubctl sync pull`, `snapshot` and `push` expose.

## How to check

1. `hubctl sync-conformance` against a running installation reports one row per client
   requirement, each passed, failed or not tested — with the reason for every one not tested.
2. It proves from the server's side that a device's identifiers are kept, that the same change
   pushed twice is applied once, that lost access produces a revocation and refuses a later push,
   that an unreadable position is refused, that a stale or invalid change is answered with its
   code, and that a field of a later version is refused by name.
3. The first-party client engine runs the same requirements through its own interface in the
   project's pipeline on every change.
4. `hubctl sync pull`, `snapshot` and `push` expose the protocol directly, with machine-readable
   output.
5. The report's rows use the same shape as the project's evidence files.

## Where it ends

* It cannot prove a client encrypts its copy; that is invisible from the server and says so.
* No certification programme; the report is the evidence.
