# Restore drill — integration, 2026-10-16

The first restore drill (RT-9) against a running installation in production's shape: integration
after its rebuild onto CloudNativePG ([ADR-0082](../adr/ADR-0082-integration-in-production-shape.md),
`deploy/integration/README.md` "Rebuilding the database"). It meets UC-BAK-08 check 6.

| | |
|---|---|
| Date | 2026-10-16, 09:48:24 – 09:53:17 UTC |
| Release | `main-6a9aa12460dc5fc050cd8af484dc63ed0a599991` (commit `6a9aa1246`) |
| Started | by hand: `kubectl -n hubtask create job --from=cronjob/hubtask-restore-drill drill-1792144103` |
| Run | `20261016-094824`, record in ConfigMap `hubtask-restore-drill` (`last_evidence`) |
| Source | cluster `hubtask-db`, one instance, schema version 159 |
| Temporary | cluster `hubtask-db-drill`, restored from the object store and torn down after the checks |
| Archive | `s3://hubtask-integration-wal/` at `nbg1.your-objectstorage.com`, **without Object Lock** |
| Result | **pass**, 9 of 9 checks |

## What it measured

| | |
|---|---|
| Recovery target | 2026-10-16T09:48:25.720Z, between the drill's two markers |
| First recoverability point | 2026-10-16T07:37:13Z (the first base backup after the rebuild) |
| Archive lag at the drill | 72 s |
| Waiting for the WAL segment | 230 s — the drill may not switch the segment by hand (`restore_drill.wal_switch_refused`), so it waited for `archive_timeout` |
| Archive failures during the drill | 0 |
| **RPO** | the archive lag, 72 s |
| **RTO** | 60 s to restore, 0.07 s to verify |

## The checks

1. `recovery_target_between_the_markers` — the first marker present, the second absent.
2. `restored_instance_promoted` — `pg_is_in_recovery` false.
3. `schema_version_matches_live` — 159 and 159.
4. `indexes_valid` — 0 invalid.
5. `constraints_validated` — 0 unvalidated.
6. `row_level_security_forced_on_tenant_tables` — 82 tenant tables.
7. `application_role_bounded` — neither superuser nor `BYPASSRLS`.
8. `no_tenant_context_sees_nothing` — 0 tenants, 0 items.
9. `tenant_sees_only_itself` — 2 workspaces judged (`operations`, `demo`).

## The node

The 8 GB node held the live cluster and the temporary one together. Sampled every 30 seconds
through the drill, the node never used more than 3,491 MiB and kept at least 4,254 MiB available,
with two clusters at once for about a minute.

## What it does not show

* **No Object Lock.** Hetzner offered none for this bucket, so the archive is not immutable: a
  stolen bucket key can delete it. Production's bucket has Object Lock (the platform's contract);
  integration's retention is the cluster's `retentionPolicy` of 35 days alone.
* **A nearly empty database.** The installation had been set up minutes before, with two workspaces
  and their owners; the 60 seconds say nothing about a restore of a large one.
* **Production.** Its own first drill stays blocked on its namespace
  (`docs/backlog/blocked-production-namespace.md`).
