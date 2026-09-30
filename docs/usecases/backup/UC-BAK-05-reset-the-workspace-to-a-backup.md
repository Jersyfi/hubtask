---
id: UC-BAK-05
title: Reset the whole workspace to an earlier backup
context: backup
actors: [PE-owner, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-02, P-03, P-04, P-08, P-11]
state: built
tasks: [E-06, H-03, F4-17]
checked_by: [core/application/service/backup/Apply_test.go, core/application/service/backup/Restore_test.go, test/backup/Restore_test.go, test/integration/restore_run_test.go]
---

# Reset the whole workspace to an earlier backup

## Goal

After a disaster inside the workspace — a script that scrambled everything, ransomware through an
integration — the owner puts the whole workspace back to how an archive recorded it, knowing
beforehand exactly what will be lost, with a way back if it goes wrong.

## Story

The owner opens *Restore*, picks the target and the archive from last night, and chooses *replace
the workspace*. First comes a rehearsal: it writes nothing and reports how many things would be
created, overwritten and removed. The screen says what the replace costs: everything since the
archive is gone, every session ends, every personal access token must be made again. The owner types
the workspace's name, confirms with their second factor, and starts it. Before anything is touched
a safety copy of the current state is written to the target. The restore runs as a job; automations
do not fire for the restored changes, reminders whose moment has passed are marked as lapsed rather
than sent, and deleted things that the deletion journal says were removed since the archive stay
removed.

## How to check

1. Only the owner can start a restore; any mode can be rehearsed first, and a rehearsal writes
   nothing and reports how many objects would be new, overwritten, skipped and removed.
2. A replace without the typed workspace name is refused with
   `backup.restore_confirmation_required`; without a fresh second-factor proof with
   `auth.step_up_required`.
3. A replace with nowhere to write the safety copy is refused with
   `backup.restore_safety_copy_unavailable`; the safety copy's name is recorded on the run before
   the replace begins.
4. The dialog states, before the owner confirms, that sessions end and tokens must be made again.
5. After a replace, the workspace holds exactly what the archive held; nothing removed for good or
   erased since the archive comes back.
6. No automation rule fires and no webhook is sent for the restored changes; reminders whose time
   has passed are *lapsed*, not sent.
7. A second restore while one is running is refused with `backup.restore_in_progress`; an archive
   written by a newer version with `backup.restore_schema_ahead`.
8. The trail is not rewritten; `backup.restore_started` and `backup.restore_finished` record the
   restore.
9. A restore can be rehearsed and run in the web app, through the API, with `hubctl restore` and
   through MCP.

## Where it ends

* No restore of sessions or tokens, ever: a copy of a credential is a credential.
* No partial undo of a replace other than restoring the safety copy.
* No restore of the whole installation from here: that mode is refused with
  `backup.restore_instance_is_the_operators`, which points at the operator's procedure.

See [backup-restore.md](../../architecture/backup-restore.md) §8.
