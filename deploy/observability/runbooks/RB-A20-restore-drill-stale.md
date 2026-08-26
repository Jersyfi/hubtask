<!-- SPDX-License-Identifier: BUSL-1.1 -->
# RB-A20 — The last restore drill is more than 90 days old

**Alert:** `HubtaskRestoreDrillStale` · **Severity:** ticket · **Catalogue:** A-20

## The symptom

Backups are being written and nobody has proved that any of them can be read back. The alert says
nothing is broken; it says nobody knows.

That is the whole point of it. A backup is a promise about a day that has not happened yet, and the
only evidence for it is a restore that worked. `POST /backups/{id}:verify` checks the checksums at
the target, which catches a truncated transfer and a rotted bit — and cannot catch an archive that
is intact and unrestorable.

## Immediate action

```bash
# What is at the target, and when was any of it last verified?
psql -c "SELECT id, target_id, status, finished_at, verified_at, verify_ok
         FROM backup_run WHERE status = 'SUCCEEDED'
         ORDER BY finished_at DESC LIMIT 10;"
```

Then run a drill: import the newest archive as a **new tenant**, which is the mode
[backup-restore.md](../../../docs/architecture/backup-restore.md) §8.2 recommends for exactly this —
it changes nothing that exists and can be thrown away afterwards.

## Why this is a ticket and not a page

Nothing is failing. What has lapsed is the evidence, and the fix is an hour of somebody's week
rather than an interruption of somebody's night. It becomes a page only in the sense that A-12 is
one: on the day it matters, it is too late to start.

## Diagnostic queries

```promql
time() - hubtask_restore_drill_last_success_timestamp_seconds   # age of the last drill
hubtask_backup_last_success_timestamp_seconds                    # is anything being written at all?
```

## Escalation

None. Do not close this without either a drill that worked or a written decision that this
installation accepts restoring untested. Both are legitimate; forgetting is not.

## Follow-up

**If this alert has never fired on an installation that has backups, check that it can.** Nothing
emits `hubtask_restore_drill_last_success_timestamp_seconds` until the restore side of milestone
`0.4.5` lands — that is why the rule does not use `absent()`, which would otherwise page every
installation for a feature that does not exist. A rule nobody has seen go red is a rule nobody
knows is connected (the reasoning behind `make gate-selftest`).
