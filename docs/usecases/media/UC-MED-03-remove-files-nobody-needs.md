---
id: UC-MED-03
title: Remove files nobody needs any more
context: media
actors: [PE-member, PE-admin, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-04, P-11]
state: built
tasks: [C-06, E-06]
checked_by: [core/application/service/media/ReconcileMedia_test.go, test/integration/media_test.go]
---

# Remove files nobody needs any more

## Goal

Files stop taking space once nothing uses them — abandoned uploads, detached attachments, replaced
covers — without a file ever vanishing from an entry that still shows it, and with a record of
every removal.

## Story

The person uploads a file and closes the tab before attaching it. Some time later the unconfirmed
upload is gone. An administrator removes a file that was attached by mistake: first they detach it
from the entry, then remove it; the removal is refused as long as any entry still carries it.

## How to check

1. An upload that was never confirmed is removed by the cleanup after a while, bytes and record.
2. A file that no entry and no cover refers to any more is removed by the cleanup, and the removal
   is written to the deletion journal.
3. Removing a file by hand is refused with `media.still_referenced` while any entry or cover refers
   to it.
4. Only the account that uploaded a file, or an administrator of the workspace, may remove it.
5. Removing by hand removes the record at once and the bytes through the cleanup, not inside the
   request.
6. A removal is in the trail as `media.deleted`, with no file name.
7. A file an entry in the trash still carries is kept until the entry itself is gone.

## Where it ends

* No file browser listing every file of the workspace.
* No undo of a removal; a removed file comes back only from a backup.
