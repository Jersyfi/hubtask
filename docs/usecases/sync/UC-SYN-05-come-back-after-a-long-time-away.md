---
id: UC-SYN-05
title: Come back after a long time away, or after a restore
context: sync
actors: [PE-person, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-11]
state: built
tasks: [N-02, N-09, N-11, P-12]
checked_by: [test/integration/sync_epoch_test.go, test/integration/initial_sync_test.go, test/integration/snapshot_test.go, packages/sync-engine/test/engine.test.ts]
---

# Come back after a long time away, or after a restore

## Goal

A device that was away longer than the server can account for — months in a drawer, or across a
restore of the workspace — does not merge a gap it cannot see. It starts over from the server's
current state, and the person sees their work as it is now.

## Story

A tablet comes out of a drawer after four months. When it connects, the server says its position is
too old. The app loads the workspace again from the start and continues from there. The same
happens to every device after an administrator restored the workspace from a backup.

## How to check

1. A device whose position is older than the offline window (90 days by default) is answered
   `sync.cursor_too_old` and loads the workspace again from the start, instead of receiving a
   partial set of changes.
2. After a restore into the workspace, every device's position is answered as too old and each
   device loads the restored state from the start.
3. A restore into a new workspace changes nothing for the devices of the old one.
4. A position the server did not issue is refused with `sync.cursor_invalid`.
5. On a position that is too old, the app drops everything it held before loading again; it never
   shows the old copy merged with the new one.
6. The full load of a large workspace can be taken as one stream, with the position to continue
   from at its end.

## Where it ends

* No merging of a gap the server cannot see: starting over is the only answer.
* No choice of what to load first; the browser loads everything the person may read.
