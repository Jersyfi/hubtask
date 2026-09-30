---
id: UC-BAK-02
title: Back up on a schedule and keep the right generations
context: backup
actors: [PE-owner, PE-selfhoster, PE-admin, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-06, P-08, P-10, P-11]
state: partial
tasks: [E-05, P-14, F4-16]
checked_by: [core/application/service/backup/Scheduling_test.go, core/application/service/backup/ScheduleLifecycle_test.go, core/application/service/backup/Pass_test.go, core/domain/model/backup/Expiry_test.go, test/backup/Run_test.go]
---

# Back up on a schedule and keep the right generations

## Goal

Once set up, the workspace is backed up by itself, at the times the owner chose in their own time
zone, and the target holds a sensible set of generations — never so few that nothing is left, never
growing without end.

## Story

The owner adds a schedule to the bucket target: every night at two, Europe/Berlin, a full backup on
Sundays and incremental ones in between. The default plan keeps the last 7, one a day for 14 days,
one a week for 8 weeks, one a month for a year and one a year for 3 years, and never fewer than 3.
After each full backup the installation opens what it just wrote and checks it restores, and says so
on the run. Old generations are removed from the target as new ones arrive — but never one a later
backup still depends on, never a file Hubtask did not write, and nothing at all after a failed run or
after a backup somebody started by hand.

## How to check

1. The owner can add, change, switch off and remove a schedule with a repeat rule and a time zone;
   an administrator or an auditor can read the schedules and their next run.
2. A schedule without a floor of generations to keep is refused with
   `backup.retention_floor_required`; the floor is at least one.
3. Scheduled runs start at the times the rule gives in the named zone, including across a daylight
   saving change.
4. After each scheduled full run, a trial restore of what was written runs and the run fails with
   `backup.trial_restore_failed` if it does not open; this is on by default and can be switched off
   per schedule.
5. Expiry keeps what the generation plan says, never fewer than the floor, never an archive a kept
   incremental needs, and never a file at the target that Hubtask did not write.
6. A failed run and a run started by hand delete nothing at the target.
7. Every archive removed at the target writes an entry in the trail.
8. Schedules — including full-backup days and the trial restore — can be created in the web app,
   through the API, with `hubctl` and through MCP.

## Where it ends

* No backup of a single hub or collection on a schedule yet; a schedule covers the workspace.
* No retry storms against a write-once target: an archive the target refuses to delete is reported
  and left.
* The retention of *business data* (what the workspace keeps) is not this; it is the lifecycle
  context's retention rules.

See [backup-restore.md](../../architecture/backup-restore.md) §5–6.

## Today

* **Check 7 fails.** Expiry removes archives with log lines only
  (`presentation/worker/BackupRun.go`); no trail entry is written, although
  [backup-restore.md](../../architecture/backup-restore.md) §6 says deletion is auditable.
* **Check 8 fails in part.** `hubctl` can list, change and remove schedules but not create one
  (`cmd/hubctl/BackupSchedule.go`); the web form sets the rule, the zone and the floor only — not
  the full-backup days, the generation plan or the trial restore.
