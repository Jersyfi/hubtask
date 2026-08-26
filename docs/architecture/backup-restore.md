# Backup and Restore

In Hubtask, backup is a **feature of the application**, not merely an operations task: targets,
schedules, and retention are configurable, existing backups at the target are listed, and restores
happen from those.

Complements [observability-reliability.md](./observability-reliability.md) §8 and
[data-protection.md](./data-protection.md). Decision:
[ADR-0019](../adr/ADR-0019-backup-targets.md).

---

## 1. Two kinds of backup

They are often conflated, but they solve different problems and have different people entitled to
them.

| | **System backup** | **Data backup (logical)** |
|---|---|---|
| Scope | The complete database plus object storage | One tenant, one hub, or one collection |
| Format | `pg_dump`/PITR base copy plus a media mirror | A Hubtask archive (§3): JSON Lines + manifest + media |
| Purpose | Total loss, server migration, ransomware | Operator error, tenant migration, export, archiving |
| Entitled | The operator (instance administrator) | Tenant `OWNER`, for their own tenant |
| Across versions | No (bound to the PostgreSQL version) | Yes (the schema version is in the manifest, migration happens on import) |
| Selective restore | No | Yes, down to item level |
| In self-hosting | Recommended | The standard |

Both use the same targets, the same scheduler, and the same retention logic. Anyone who wants only
one variant runs only one.

---

## 2. Backup targets

The target is a port (`core/port/backupstorage/Port.go`) with interchangeable adapters. No target is
preferred, and none is a prerequisite.

The four the roadmap opens with exist (E-03); the rest ship when they pass the same conformance
suite, which is what ADR-0019 decision 2 means by a gate rather than an aspiration. A target of a
kind this build has no adapter for is refused with `backup.kind_unsupported` — "Hubtask cannot talk
to SMB yet" rather than "SMB is not a thing".

| Adapter | Built | Protocol / notes |
|---|---|---|
| `local` | yes | A directory inside the installation's backup volume (`HUBTASK_BACKUP_LOCAL_PATH`, the self-hosting default). A target's own path is **relative** to that volume and cannot leave it: whoever configures a target administers the instance, not the machine |
| `s3` | yes | S3-compatible: AWS, MinIO, Ceph, Wasabi, Backblaze B2, Hetzner, IDrive e2 — the endpoint is free; server-side encryption and object lock usable. An archive of unknown length is uploaded in parts, so the process holds one part rather than an archive |
| `sftp` | yes | SSH-based, password or key. The host key is **configuration**: a target names the server's public key or its SHA-256 fingerprint, and one that names neither is refused. There is no trust on first use and no way to switch the check off — a target is created through an API, and a first connection that accepted whatever answered is one an attacker only has to be present for once |
| `ftps` | — | FTP over TLS (explicit) |
| `ftp` | — | Only with explicit confirmation — unencrypted transport, a warning in the UI/API, and an audit entry |
| `webdav` | yes | Nextcloud, ownCloud, generic WebDAV servers. Listed by recursing `PROPFIND` at depth one rather than asking for infinite depth, which Apache refuses by default |
| `smb` | — | Windows/NAS shares |
| `azure_blob`, `gcs` | — | Through the respective S3-compatible or native API |
| `rclone` (optional) | — | An umbrella adapter for Dropbox, Google Drive, OneDrive, pCloud and others, when `rclone` is available in the image. Gated additionally on open point B-1 |
| `http_put` | — | A generic target for home-grown solutions |

Several targets in parallel are explicitly provided for (the 3-2-1 rule: local + remote + a
different provider). Each target has its own schedule and its own retention.

**The target configuration is an exception to the SSRF rule** from [security.md](./security.md) —
and a deliberately narrow one:

* Backup targets may **only** be created by instance administrators, not by arbitrary tenant users. In practice that is the owner's right in the role matrix: in single-tenant operation the tenant's owner *is* the instance administrator, and in provider operation the operator can allow tenants their own targets (`HUBTASK_BACKUP_TENANT_TARGETS=true`, off by default) — an egress allowlist then applies on top.
* **Every** call to a target runs through the same `GuardedClient`, not only the connection test: metadata endpoints, RFC 1918 ranges and loopback are refused unless `HUBTASK_HTTP_ALLOW_PRIVATE_NETWORKS` releases them, and no redirect is followed. SSH is not HTTP, so the SFTP adapter uses the guard's resolver and dial-time control directly rather than the client. Gate BK-9 is that sentence as a test.
  The consequence is worth stating plainly, because self-hosters hit it: a MinIO or a NAS on the same LAN needs that release. It is a decision an operator makes once for the installation rather than one every target gets for free.
* Creating or changing a target is auditable (`backup.target_changed`), because a backup target is by definition a data egress channel. The entry records where the data may now go — the kind, the configuration, the encryption mode — and never the credential.
* Credentials are sealed with the envelope of E-02, bound to the row they belong to, and are read back by exactly one repository method. The statements that feed a response do not select the column, so a credential cannot reach a client because somebody added a field to a mapper.

---

## 3. The archive format

A Hubtask archive is a directory or `tar` stream with a fixed structure:

```
hubtask-backup-<tenant>-<utc-timestamp>-<full|incremental>/
├── manifest.json          # format version, schema version, product version, scope,
│                          # period, counts, encryption, checksums, parent_id
├── data/
│   ├── containers.jsonl
│   ├── work_items.jsonl
│   ├── comments.jsonl
│   ├── labels.jsonl … automation_rules.jsonl, saved_views.jsonl, templates.jsonl
│   └── audit.jsonl        # optional, see §7
├── media/<sha256-prefix>/<sha256>   # content-addressed, deduplicated
└── checksums.txt
```

Properties:

* **JSON Lines rather than an SQL dump**, so that an archive from version 1.2 stays readable in version 1.7: on import, the same upward migrations run as for domain objects. A `pg_dump` would be tied to the PostgreSQL and schema version.
* **Content-addressed media**, so that incremental runs do not re-transfer unchanged files.
* **Incremental** based on `updated_at`/`seq` against a parent archive; deletions are carried as tombstones, otherwise deleted objects would come back on restore.
* **Encrypted** (§4) — before it leaves the process, not just at the target.
* **Checksums** per file and over the manifest; `POST /backups/{id}:verify` checks an archive at the target without restoring it.

---

## 4. Encryption

Backups sit, by definition, on somebody else's storage. Therefore:

* **Client-side encryption is the standard**, not an option: AES-256-GCM with a backup key per target (derived from a passphrase via Argon2id, or supplied directly).
* The key is **not** stored in the archive. Without it the backup is useless — this is stated as an unmissable notice during setup and is logged on confirmation.
* Optionally, server-side encryption at the target on top (S3 SSE) — it does not replace our own.
* The key can be rotated; old archives stay readable with the old key (the key ID is in the manifest).
* An unencrypted backup is possible (`encryption: none`), but it requires explicit confirmation and produces a permanent warning in `/meta/health`.

---

## 5. Schedule and execution

Schedules are RRULE-based — the same mechanism as recurring tasks, not a second scheduling system:

```json
{
  "target_id": "…",
  "scope": { "kind": "TENANT", "id": "…" },
  "schedule": "FREQ=DAILY;BYHOUR=3;BYMINUTE=0",
  "timezone": "Europe/Berlin",
  "mode": "INCREMENTAL",
  "full_every": "FREQ=WEEKLY;BYDAY=SU",
  "retention": { "keep_last": 7, "keep_daily": 14, "keep_weekly": 8, "keep_monthly": 12, "keep_yearly": 3, "min_keep": 3 },
  "encryption": { "mode": "AES256_GCM", "key_id": "bk_2026_a" },
  "include_media": true,
  "include_audit": true,
  "notify_on": ["FAILURE", "FIRST_SUCCESS_AFTER_FAILURE"]
}
```

Execution is an ordinary job (the `worker` role) with progress, the ability to cancel, resumption
after process death, and a lock against parallel runs per target. A running backup job must not slow
down interactive operation: reads go through a replica or at a throttled rate, on a bulkhead pool
separate from the API path.

**Consistency:** the export runs in a transaction with a `REPEATABLE READ` snapshot, so that the
archive represents a consistent point in time rather than a mixture of before and after. Media are
fetched after the snapshot, using the referenced checksums.

---

## 6. Retention of backups

Two levels, deliberately kept separate from the retention of business data
([data-retention.md](./data-retention.md)):

1. **The generation principle** (`keep_last`, `keep_daily`, `keep_weekly`, `keep_monthly`, `keep_yearly`) — as in established backup tools.
2. **`min_keep`** as a floor: a retention rule may never result in **no** backup being left. If a run fails, old archives are not deleted.

Expiry rules apply only to archives Hubtask created itself (recognised by the manifest); other files
at the target are never touched. Deletion is auditable. At targets with object lock/WORM, Hubtask
reports non-deletable archives as a notice instead of retrying endlessly.

---

## 7. Backup and data protection

The conflict is well known: an erasure request takes effect immediately in the primary system, but
last week's archive still contains the data. How it is handled:

* Deletions are recorded in a **deletion journal**. On restore they are reapplied: objects deleted between the archive point and the restore do not come back. This is the most effective measure, because it works without access to old archives.
* The retention period of the backups is the effective upper bound on deletion; it is documented in the data catalogue and is made transparent to data subjects rather than concealed.
* `include_audit` is configurable: including the audit trail gives better evidence, but longer persistence of personal metadata.
* Downloading an archive is itself an auditable data access (`backup.downloaded`).
* An archive that leaves the server falls under the operator's responsibility for the chosen target — with third-country targets that is a transfer within the meaning of GDPR Chapter V. Hubtask points this out during target setup and records the region in the target record.

---

## 8. Restore

### 8.1 Browsing the target

`GET /backup-targets/{id}/backups` lists the archives present at the target without requiring any
state in the database — the list is read from the manifests at the target. That means a restore
works even when the database is lost and only the target credentials exist. Shown are the timestamp,
scope, size, full/incremental, the chain to the parent archive, the checksum status, and the
encryption key ID.

### 8.2 Modes

| Mode | Effect | Typical occasion |
|---|---|---|
| `INSPECT` | Read the archive, show a content overview and the difference against the current state; changes nothing | "What if" |
| `SELECTIVE` | Pull selected containers/items back into the existing tenant | An accidentally deleted collection |
| `MERGE` | Import the archive, handling existing objects by rule (`skip`, `overwrite`, `duplicate`) | Merging, partial loss |
| `REPLACE_TENANT` | Reset the tenant entirely to the archive state | A serious error, ransomware |
| `NEW_TENANT` | Import the archive as a new tenant | Migration, a test copy, forensics |
| `INSTANCE` | Import a system backup (operator, maintenance mode) | Total loss |

`NEW_TENANT` is the recommended way to check before a destructive mode: import alongside first, look
at it, then decide.

### 8.3 The procedure

1. Pre-check: checksums, schema/product version, decryptability, scope, estimated duration.
2. A dry run with a report (the number of new/overwritten/skipped objects, conflicts).
3. Confirmation by typing the tenant name for destructive modes; plus step-up authentication.
4. An automatic safety copy of the current state before destructive modes (if there is room at the target).
5. Execution as a job with progress; on cancellation, rollback within a transaction per batch size.
6. Follow-up: apply the deletion journal (§7), rebuild the search index, do **not** re-fire automation for the period (§8.4), and write the report to the audit.

### 8.4 What deliberately does *not* happen during a restore

* **No automation rules fire.** A restore would otherwise trigger hundreds of webhooks and emails effectively reporting old states. Restored changes produce events with `replay: true`, which the rule engine ignores.
* **No reminders are caught up** whose time lies in the past; they are marked as lapsed.
* **No webhooks are re-delivered**; the archive's outbox is not imported.
* **No tokens or sessions are restored** — making credentials from an archive valid again is a security risk. Users must sign in again; PATs must be recreated. This is displayed before the restore.

---

## 9. Import and export of existing systems

Separate from backup, but technically related: the importer for Trello, Microsoft To Do, Google
Tasks, and CSV (roadmap `0.9.0`) produces the same internal intermediate form as an archive. That
gives one ingestion path, not two.

The user export (`GET /tenants/{id}:export`, GDPR portability) also produces a Hubtask archive —
unencrypted or password-protected, directly downloadable. An export is therefore simultaneously a
restorable backup, without a second format coming into existence.

---

## 10. Self-diagnosis and alerts

Complements the catalogue in [observability-reliability.md](./observability-reliability.md):

Two vocabularies, and E-03 settled which is which rather than leaving them to drift: a warning is
named after what it describes. `backup.target_*` is a warning **a target carries about itself** and
travels in the `warnings` array of the resource; `config.backup_*` is a warning about **the
installation** and belongs in the health report. They are not synonyms and neither is a rename of
the other.

| Signal | Meaning |
|---|---|
| `backup.target_unencrypted` (on the resource) | This target stores archives unencrypted |
| `backup.target_plaintext_protocol` (on the resource) | This target is reached over a connection anybody on the wire can read. Judged by the scheme in the configuration for every kind addressed by a URL, and by the name only for `ftp`, which has no secure form |
| `config.backup_not_configured` (a warning in `/meta/health`) | No target configured |
| `config.backup_unencrypted` | A target without encryption |
| `config.backup_single_target` | Only one target — a pointer to 3-2-1 |

The three `config.backup_*` warnings have their message codes and the repository count behind them
(E-03) and no surface yet. `/meta/health` on the operations port is process-wide, and a backup
target is a row in a tenant's database behind row level security — a count taken there sees
nothing. They arrive with the tenant-facing health report, which is still
`route.operation_not_available`. Until then they were **not** derived from environment variables:
the previous condition read `HUBTASK_BACKUP_LOCAL_PATH` and `HUBTASK_BACKUP_TARGETS`, neither of
which said whether a target exists, and the second was read by nothing else and documented nowhere.
It is removed.
| `backup.last_success_age_hours` (metric) | The age of the last successful backup per target |
| `backup.verify_failed_total` | A checksum error at the target → a damaged archive |
| `backup.restore_test_age_days` | The time since the last verified restore |
| A-12 (tightened) | The last successful backup is older than twice the schedule interval |
| A-19 (new) | The restore drill is older than 90 days |

The last point is deliberate: a backup that has never been restored is a hypothesis. The regular
`NEW_TENANT` trial restore can be automated and evaluated as a test run.

---

## 11. Evidence

| Test | Contents |
|---|---|
| BK-1 | A round trip per adapter (local, s3, sftp, webdav) against a test container: back up, list, verify, restore |
| BK-2 | An encrypted archive is unreadable without the key; with a rotated key, the old archive stays readable |
| BK-3 | An incremental chain over 10 runs including deletions reproduces the source state exactly |
| BK-4 | An archive from an older schema version imports correctly (golden archives in the repository, one per major version) |
| BK-5 | A restore triggers no automation, sends no webhooks or emails, and restores no tokens |
| BK-6 | The deletion journal prevents deleted objects from returning |
| BK-7 | Process death during a backup and during a restore: resumption without duplicates |
| BK-8 | Retention deletes according to the generation plan, `min_keep` is never undercut, and other files at the target stay untouched |
| BK-9 | A target configuration pointing at an internal address is blocked by `GuardedClient` unless explicitly released |
| BK-10 | Cross-tenant: tenant A cannot list, verify, or restore an archive belonging to B |

---

## 12. Open points

| # | Point | Needed by |
|---|---|---|
| B-1 | Whether `rclone` goes into the image (size, and its GPL-3.0 licence — check distribution alongside BSL) | `0.5.0` |
| B-2 | Whether system backups (PITR) are orchestrated by Hubtask or left to the operator | `0.6.0` |
| B-3 | Retention protection against ransomware (recommend object lock as mandatory?) | `0.6.0` |
| B-4 | The scope of the trial restore in the default schedule | `0.9.0` |
