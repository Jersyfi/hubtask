---
id: UC-SYN-04
title: Lose access, and have my device's copy follow
context: sync
actors: [PE-member, PE-guest, PE-admin]
deployments: [D2, D3, D4, D5, D6, D7]
serves: [P-01, P-05, P-11]
state: built
tasks: [N-08, N-09]
checked_by: [test/integration/revocation_test.go, test/integration/sync_retention_test.go, packages/sync-engine/test/replica.test.ts]
---

# Lose access, and have my device's copy follow

## Goal

When somebody's access to part of a workspace ends — they leave the project, a share is withdrawn,
a hub moves out of their reach — the copies on their devices lose that part too, and nothing they
change offline in it is accepted afterwards.

## Story

The administrator removes Ben from the *Clients* hub. Ben's laptop was offline; when it reconnects,
the *Clients* hub and everything under it disappear from his copy. The edit he had made there
offline is refused, and his app lists it as a refused change instead of pretending it was saved.
Ben keeps a second path to one collection through a group; that collection stays.

## How to check

1. When a person's effective read access to a hub or collection ends — a membership revoked, a
   group left, a group deleted, a share withdrawn, a collection moved out of reach — their devices
   receive a revocation for that root and remove everything under it from the copy.
2. A person who still reads the same part through another path receives no revocation.
3. The revocation reaches only the person it concerns.
4. A change pushed afterwards to something the person may no longer touch is refused, and the app
   shows it as a refused change.
5. An entry that was deleted for good answers `sync.gone` to a device still holding it; the app
   drops the change and does not recreate the entry.
6. A deletion reaches every device as a deletion within the offline window (90 days by default), so
   a device returning within it cannot bring a deleted entry back.

## Where it ends

* The server cannot erase a copy on a device that never reconnects; forgetting the device is what
  stops it.
* Nothing reaches a device from a workspace the person is not in ([P-01](../../vision/principles.md)).
