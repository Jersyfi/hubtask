<!-- SPDX-License-Identifier: BUSL-1.1 -->
# RB-A12 — No successful backup in the last 24 hours

**Alerts:** `HubtaskBackupStale`, and A-12's point-in-time recovery half — `HubtaskArchiveGap`,
`HubtaskArchiveFailing`, `HubtaskBaseBackupStale`, `HubtaskNoRecoverabilityPoint`,
`HubtaskReplicationLag` · **Severity:** page · **Catalogue:** A-12

**Which half fired decides which section applies.** `HubtaskBackupStale` is about the *tenant
archive backups* this application writes, and everything down to "Follow-up" is about it. The five
alerts of `prometheus-rules-pitr.yaml` are about the *system backup* — the database's continuous
WAL archiving — and their section is at the bottom. They are one catalogue entry because they are
one question, "can this installation still get its data back", and two different mechanisms answer
it for two different parties (`backup-restore.md` §1).

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

---

## The point-in-time recovery half (H-10)

These fire where the database is the CloudNativePG cluster this chart owns
([deployment.md §3.2](../../../docs/architecture/deployment.md#32-where-production-runs)). They read
the operator's own metrics, not the application's, and the recovery they are about is the
operator's procedure in
[backup-restore.md §8.5](../../../docs/architecture/backup-restore.md#85-the-operator-procedure-point-in-time-recovery).

**All five say one thing: the recovery window has stopped moving with the database.** Nothing is
broken for anybody using the system right now, and that is exactly why they are a page — the cost
becomes visible on the day somebody needs a recovery, and by then the window is where it stopped.

```bash
# What the operator thinks of the cluster, and where its archive stands.
kubectl -n <namespace> get cluster
kubectl -n <namespace> describe cluster <cluster> | sed -n '/Status:/,$p'

# The archiver, from inside the primary.
kubectl -n <namespace> exec <cluster>-1 -c postgres -- \
  psql -qAt -c "SELECT last_archived_wal, last_archived_time, failed_count, last_failed_wal,
                       last_failed_time FROM pg_stat_archiver"

# And what is queueing to be archived.
kubectl -n <namespace> exec <cluster>-1 -c postgres -- \
  sh -c 'ls -1 "$PGDATA"/pg_wal/archive_status/*.ready 2>/dev/null | wc -l'
```

| Alert | What it means | Where to look first |
|---|---|---|
| `HubtaskArchiveGap` | Segments are piling up unarchived — the window is falling behind by that much | The archiver's `last_failed_wal`, then the bucket's reachability and its policy |
| `HubtaskArchiveFailing` | The archiver is returning errors | The backup credentials' Secret, the endpoint, and whether the bucket's Object Lock retention still admits writes |
| `HubtaskBaseBackupStale` | The daily base backup has not succeeded for two days | `kubectl -n <namespace> get backup` — the newest one's phase and its error |
| `HubtaskNoRecoverabilityPoint` | No base backup has *ever* completed | The `ScheduledBackup` exists and its first immediate run failed; read that Backup object |
| `HubtaskReplicationLag` | A replica is past the recovery point objective | Only where a replica exists; the decided shape is one instance |

**The credential is the usual cause and the trap.** The writer must be able to write and must
*not* be able to delete or shorten retention ([ADR-0046](../../../docs/adr/ADR-0046-production-on-a-platform-namespace.md),
B-3). A policy narrowed past that boundary breaks archiving in exactly this way, and the fix is the
policy rather than a wider credential.

**Do not close any of these by widening the retention.** The lock retention and the backup plan's
own retention have to stay equal (35 days, P-5); raising one to silence an alert breaks the promise
`data-protection.md` §12 makes about deletion.

## After it is fixed

Run the drill rather than assuming: it is the only thing that answers "does this archive restore".

```bash
kubectl -n <namespace> create job --from=cronjob/<release>-restore-drill drill-$(date +%s)
kubectl -n <namespace> logs -l app.kubernetes.io/component=restore-drill --tail=50
```

A pass moves `last_success` in the drill's record ConfigMap, which is what feeds
`hubtask_restore_drill_last_success_timestamp_seconds` and silences
`HubtaskRestoreDrillNeverRan` — see [RB-A20](./RB-A20-restore-drill-stale.md).
