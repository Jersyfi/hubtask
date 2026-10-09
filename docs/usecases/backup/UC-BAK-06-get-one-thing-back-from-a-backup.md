---
id: UC-BAK-06
title: Get one lost collection or task back from a backup
context: backup
actors: [PE-owner, PE-admin, PE-person, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-08, P-11]
state: partial
tasks: [E-06, F4-17, PH-10]
checked_by: [core/application/service/backup/Apply_test.go, core/application/service/backup/Restore_test.go, test/integration/backup_import_test.go]
---

# Get one lost collection or task back from a backup

## Goal

Something that is gone — emptied from the trash, or ruined by an edit weeks ago — comes back from a
backup on its own, without resetting the rest of the workspace, either in its old place or beside
what is there now.

## Story

A family emptied the trash in March and in May misses the *Holiday 2026* collection. The parent opens
*Restore*, picks an archive from February, and chooses the collection *from the archive*. The
rehearsal says what would come back. They choose whether things that still exist are skipped,
overwritten or duplicated; with *duplicate*, the collection comes back beside the current one as
*Holiday 2026 (restored 2026-05-12 a1b2c3)*, with its own copies of its lists, labels and tasks,
and renaming it afterwards is an ordinary edit. Everything else in the workspace is untouched.

## How to check

1. An owner can restore chosen collections or tasks from an archive into the current workspace,
   leaving everything not chosen untouched; without a selection the request is refused with
   `backup.restore_selection_required`.
2. The choice is made from what the archive holds — including hubs, collections and tasks that no
   longer exist in the workspace.
3. Choosing a collection brings back everything under it and everything that hangs off it
   (buckets, labels, tasks, comments).
4. With *skip*, existing things are left; with *overwrite*, the archive's version replaces them,
   except the workspace's legal holds and an account's restriction of processing, which stay as
   they are;
   with *duplicate*, a copy lands beside them and the top of the copied tree is named
   `<name> (restored <date> <six characters>)`.
5. Restoring the same archive twice with *duplicate* lands a second copy with a different suffix;
   a resumed restore does not create a second copy of what it already wrote.
6. Accounts, media and webhook subscriptions are never duplicated; the report says where
   *duplicate* fell back to *skip*.
7. The rehearsal reports what would come back before anything is written.
8. A selective restore can be made in the web app, through the API, with `hubctl` and through MCP.

## Where it ends

* No restore of a single field or a single edit; the unit is an object.
* No browsing of an archive's content in the web app beyond choosing what to restore.
* No merging of two versions of one task; one of them wins, or both exist.

See [backup-restore.md](../../architecture/backup-restore.md) §8.2.

## Today

* Check 2: not met in the web app — the selection offers only hubs that exist in the workspace today; a collection, a task or a hub that is gone cannot be chosen.
* Check 3: not proven — the server computes the closure of a selection, but nothing restores a selection and inspects what landed.
* Check 8: not met for `hubctl` — `hubctl restore run` has no selection flags.
