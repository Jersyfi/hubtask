<!-- SPDX-License-Identifier: BUSL-1.1 -->
# RB-A12 — No successful backup in the last 24 hours

**Alert:** `HubtaskBackupStale` · **Severity:** page · **Catalogue:** A-12

## The symptom

Either no backup has succeeded in 24 hours, or **the backup metric is absent entirely** — which
means no backup is configured at all. The alert is deliberately written to fire on the absence:
an installation that never had a backup is the one that most needs telling.

Nothing is broken right now. That is exactly why it is a page: the cost of this alert becomes
visible only on the day the disk fails, and by then it is not fixable.

## Immediate action

```bash
# Is a backup target configured at all?
psql -c "SELECT id, name, kind, enabled, last_test_at, last_test_ok FROM backup_target;"
psql -c "SELECT status, started_at, finished_at, error_code, size_bytes
         FROM backup_run ORDER BY started_at DESC LIMIT 10;"
```

* **No target rows** → nothing was ever set up. `/meta/health` has been carrying the
  `config.backup_not_configured` warning since the first start. Set up a target
  ([backup-restore.md](../../../docs/architecture/backup-restore.md)).
* **Runs with `status = 'FAILED'`** → `error_code` names the cause: unreachable target, wrong
  credentials, no space.
* **No runs at all despite a target** → the schedule is missing or the `scheduler` role is not
  running (see [RB-A03](./RB-A03-not-ready.md)).

## The check that actually matters

A backup that has never been restored is a hope, not a backup. After fixing this alert, run a
restore into a scratch database — a restore drill is a release criterion in this project, not a
nice-to-have (observability-reliability.md §8).

## Diagnostic queries

```promql
time() - hubtask_backup_last_success_timestamp_seconds   # age per target, which is what alerts
absent(hubtask_backup_last_success_timestamp_seconds)     # 1 = nothing is reporting at all
```

The gauge is labelled by target, and the rule deliberately does not aggregate: with the 3-2-1
arrangement [backup-restore.md](../../../docs/architecture/backup-restore.md) §2 recommends, a
`max()` across targets would let the local copy hide a remote one that has been failing for a week.
The `target_id` label on the firing series is the one to look at.

## Escalation

None, but do not close this without either a working backup or a written decision that this
installation accepts total data loss. Both are legitimate; forgetting is not.

## Follow-up

The metric exists since `0.4.5` (E-05) and is published by the `scheduler` role. If it is absent on
a version that has backups, the leader is not running — see [RB-A03](./RB-A03-not-ready.md) — rather
than "backups are not implemented", which was the honest state before that release.

And then [RB-A20](./RB-A20-restore-drill-stale.md): a backup that works is not the same as a backup
that restores, and the second is the one that matters.
