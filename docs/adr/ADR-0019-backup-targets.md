# ADR-0019: Backup as an application feature with freely chosen targets

* **Status:** accepted
* **Date:** 2026-08-14
* **Concerns:** operations, data, security
* **Related:** [ADR-0003](./ADR-0003-postgresql-as-single-datastore.md), [ADR-0015](./ADR-0015-security-baseline.md), [backup-restore.md](../architecture/backup-restore.md)

## Context

Backup should be configurable: any target (S3, FTP, SFTP, WebDAV, cloud storage, local), any
schedule, retention rules, a listing of the backups present at the target, and restore from those —
automatically, without tools outside the application.

That sits in tension with two existing decisions: operation should stay trivial for private users
(only PostgreSQL as a mandatory dependency), and outbound connections are strictly limited because
of SSRF ([ADR-0015](./ADR-0015-security-baseline.md)). A freely configurable backup target is by
definition a data egress channel.

On top of that: the obvious route — pushing a `pg_dump` to a target — is tied to the PostgreSQL and
schema version, allows no selective restore of individual objects, and cannot be narrowed to one
tenant in multi-tenant operation.

## Decision

1. **Backup is a feature of the application**, not purely an operations task: targets, schedules, retention, listing, and restore all run through the API and jobs.
2. **The target is a port** (`core/port/backupstorage`) with equal-ranking adapters (local, s3, sftp, ftps, ftp, webdav, smb, azure, gcs, optionally rclone, http\_put). No provider is preferred; several targets in parallel are provided for.
3. **Two kinds of backup:** a logical data backup in the Hubtask archive format (JSON Lines + manifest + content-addressed media, importable across versions, selective down to item level) and an optional system backup (`pg_dump`/PITR) for the operator.
4. **Client-side encryption is the standard** (AES-256-GCM, with the key not in the archive). An unencrypted backup requires explicit confirmation and produces a permanent warning in the self-diagnosis.
5. **Targets are the operator's business.** Creating and changing one requires instance administrator rights; tenant-owned targets are an optional feature with an additional egress allowlist. Every target change is auditable, and every connection goes through the `GuardedClient`.
6. **Restore reads the listing from the manifests at the target**, not from the database — otherwise it would be useless in a total loss.
7. **A restore fires no automation, sends no notifications, and restores no tokens.**

## Options considered

| Option | Assessment |
|---|---|
| **Chosen: a port with many adapters plus our own archive format** | Meets the requirement fully; the effort lies in maintaining the adapters and the format. |
| Documentation only ("use pg_dump and restic") | The cheapest variant, but it misses the requirement: no listing, no selective restore, and practically out of reach for private users. |
| `pg_dump` to configurable targets only | Simpler, but version-bound, not tenant-scoped, not selective, and no restore of individual objects. |
| Embedding a third-party tool (restic/borg as a requirement) | Mature deduplication and encryption, but another mandatory dependency, GPL questions around distribution, and no access to the business structure (no selective recovery of a collection). It remains possible as an optional adapter. |
| Supporting S3 only | Covers the majority of providers, but misses explicitly requested targets such as FTP and private cloud storage. |

## Consequences

**Positive**

* A private user sets up a backup to their own storage without the command line.
* A restore works even when only the target credentials remain.
* Selectively recovering an accidentally deleted collection is possible — by far the most common real occasion.
* Export (GDPR portability), backup, and import use the same format; three separate paths do not appear.

**Negative / countermeasures**

* *Many adapters mean maintenance effort and a test matrix.* → One shared conformance test (BK-1) runs against every adapter with test containers; new adapters must pass it or they do not ship.
* *Our own archive format is a long-term commitment.* → The format version in the manifest, golden archives per major version in the repository, and an import test against all of them (BK-4).
* *Free targets soften the egress hardening.* → Restriction to instance administrators, the `GuardedClient`, an audit obligation, an optional allowlist; in provider operation, tenant-owned targets are off by default.
* *Encryption shifts the risk onto the key.* → An unmissable notice at setup, a logged confirmation, and rotation without losing old archives.
* *FTP is insecure.* → It is supported because it was explicitly requested, but only with explicit confirmation, a warning in the self-diagnosis, and an audit entry.
* *Backups effectively extend the deletion deadline.* → The deletion journal on restore, documented retention in the data catalogue, and transparency towards data subjects ([backup-restore.md](../architecture/backup-restore.md) §7).
