---
id: UC-LIF-06
title: Place a legal hold so that nothing is destroyed
context: lifecycle
actors: [PE-owner, PE-auditor, PE-scripter]
deployments: [D3, D4, D5, D6, D7]
serves: [P-03, P-07, P-08, P-11]
state: partial
tasks: [PH-01, E-08, F4-18]
checked_by: [core/application/service/lifecycle/LegalHolds_test.go, core/domain/model/lifecycle/LegalHold_test.go, test/integration/legal_hold_test.go, test/retention/retention_test.go, cmd/hubctl/Hold_test.go]
---

# Place a legal hold so that nothing is destroyed

## Goal

When a dispute, an inspection or a lawsuit makes it necessary, the owner freezes the destruction of
a workspace, a hub or collection, or a single task: from that moment nothing under the hold is
removed for good — not by a person, not by a rule, not by the trash — until the hold is released.

## Story

The company's lawyer asks that nothing about the *Supplier X* project be destroyed. The owner places
a legal hold on its hub, with a reason. People keep working: they can still edit, delete to the
trash, restore and archive. But a *delete for good* on anything in the hub is refused and says a
legal hold is in force; emptying the trash leaves those items and reports them as kept; retention
rules pass over them. When the case is closed, the owner releases the hold with a reason. The hold
stays on record, released, for whoever audits it later.

## How to check

1. Only the owner can place or release a hold; an administrator or an auditor sees the list of
   holds, placed and released.
2. A hold names its scope (the workspace, a hub or collection, or one task) and a reason; without a
   reason it is refused with `lifecycle.hold_reason_required`, and releasing needs a reason too.
3. Deleting for good anything under a hold is refused with `lifecycle.legal_hold` — the workspace
   itself included, and resetting the workspace to a backup while a hold is in force; a reset
   that runs keeps the workspace's holds as they are, never the backup's.
4. Emptying the trash leaves everything under a hold in place and reports it as kept because of a
   legal hold.
5. No retention rule and no automatic trash purge removes anything under a hold.
6. Editing, moving to the trash, restoring and archiving remain possible under a hold.
7. A hold is never deleted; a released hold stays listed with both reasons and moments. Placing and
   releasing write `lifecycle.hold_placed` and `lifecycle.hold_released`.
8. The web app offers only the scopes a hold can be placed on.

## Where it ends

* A hold on one person's data (an account) is decided (R-3, 2026-09-30,
  [data-protection.md](../../architecture/data-protection.md) §4.1) and not built yet: it covers
  that person's contributions and the account, and stops their erasure, not their sign-in. Until
  PH-01 it is refused with `lifecycle.hold_account_scope_unavailable`.
* A hold does not freeze editing; preserving the state at one moment is a backup's job.
* A hold does not stop backups from expiring at their target.
* A workspace already pending deletion when a hold is placed stays pending until the last hold is
  released; its removal within 65 days ([data-protection.md](../../architecture/data-protection.md)
  §5) waits for that.

See [data-retention.md](../../architecture/data-retention.md) §4 and
[ADR-0020](../../adr/ADR-0020-retention-policies.md).

## Today

* Check 3: not met — erasing a person removes data under a hold, tracked in #1086; deleting the workspace and resetting it to a backup read no hold, and the reset replaces the holds with the backup's, tracked in #1228.
* Check 8: not met — the web app's scope picker offers *account*, which the server refuses, tracked in #1086.
