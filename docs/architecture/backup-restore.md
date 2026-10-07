# Backup and Restore

Backup is a **feature of the application**, not merely an operations task: targets, schedules and
retention are configurable, the archives at a target are listed, and restores run from those.
Decision: [ADR-0019](../adr/ADR-0019-backup-targets.md). Related:
[observability-reliability.md](./observability-reliability.md) §8,
[data-protection.md](./data-protection.md), [tenant-export.md](./tenant-export.md).

---

## 1. Two kinds of backup

| | **System backup** | **Data backup (logical)** |
|---|---|---|
| Scope | The complete database plus object storage | One tenant, one hub, or one collection |
| Format | PITR from the WAL archive, or a `pg_dump`, plus a media mirror | A Hubtask archive (§3): JSON Lines + manifest + media |
| Purpose | Total loss, server migration, ransomware | Operator error, tenant migration, export, archiving |
| Entitled | The operator (instance administrator) | Tenant `OWNER`, for their own tenant |
| Across versions | No (bound to the PostgreSQL version) | Yes (the schema version is in the manifest; migration happens on import) |
| Selective restore | No | Yes, down to item level |
| Who performs it | The database operator (CloudNativePG) or the operator by hand — never Hubtask (§8.5, §8.6) | Hubtask, through the API and jobs |
| In self-hosting | Recommended (§8.6) | The standard |

---

## 2. Backup targets

The target is a port (`core/port/backupstorage/Port.go`) with interchangeable adapters
(`infrastructure/backupstorage`); none is preferred or a prerequisite. A kind with no adapter in
this build is refused with `backup.kind_unsupported`. An adapter ships when it passes the
conformance suite (BK-1).

| Adapter | Built | Protocol / notes |
|---|---|---|
| `local` | yes | A directory inside the installation's backup volume (`HUBTASK_BACKUP_LOCAL_PATH`, the self-hosting default). A target's path is **relative** to that volume and cannot leave it: whoever configures a target administers the instance, not the machine |
| `s3` | yes | Any S3-compatible endpoint; server-side encryption and object lock usable. An archive of unknown length is uploaded in parts, so the process holds one part, not an archive |
| `sftp` | yes | SSH, password or key, through an in-house SFTP v3 client over `golang.org/x/crypto/ssh` — no SFTP library ([security.md](./security.md) §11). The host key is **configuration**: a target names the server's public key or its SHA-256 fingerprint, and one that names neither is refused. No trust on first use, no way to switch the check off |
| `webdav` | yes | Nextcloud, ownCloud, generic WebDAV. Listed by recursing `PROPFIND` at depth one, not infinite depth (which Apache refuses by default) |

Not built: `ftps`; `ftp`, only with explicit confirmation, a warning in the UI/API and an audit
entry; `smb`; `azure_blob`, `gcs`; `rclone`, an umbrella adapter when it is in the image (B-1);
`http_put`. Several targets in parallel are intended (3-2-1: local + remote + a different
provider), each with its own schedule and retention.

**The target configuration is a narrow exception to the SSRF rule** of [security.md](./security.md):

* **Who may create a target:** the owner's right in the role matrix. In single mode the tenant's owner
  *is* the instance administrator. In provider operation tenants may have their own targets only when
  the operator sets `HUBTASK_BACKUP_TENANT_TARGETS=true` (off by default); an egress allowlist then
  applies on top. Otherwise creation answers `backup.tenant_targets_disabled`.
* **Every call to a target** goes through `GuardedClient` (SFTP through its resolver and dial-time
  control): metadata endpoints, RFC 1918 ranges and loopback are refused unless
  `HUBTASK_HTTP_ALLOW_PRIVATE_NETWORKS` releases them — for a store on the same LAN, the operator's
  decision — and no redirect is followed (BK-9).
* **Creating or changing a target is audited** (`backup.target_changed`): the kind, the
  configuration, the encryption mode — never the credential.
* **Removing a target** removes it from the configuration and touches nothing at the target; the
  archives there remain (`backup.target_removed`).
* **Credentials are sealed** with the envelope encryption of [security.md](./security.md) §8, bound to
  their row, and read back by exactly one repository method; the statements that feed a response do
  not select the column.

---

## 3. The archive format

A Hubtask archive is specified in [tenant-export.md](./tenant-export.md) §3–§8 — the container,
the manifest, `checksums.txt` as the commit point, content-addressed media, verification. A backup
is named `hubtask-backup-<tenant>-<utc-timestamp>-<full|incremental>`. What the backup adds:

* **JSON Lines, not an SQL dump**: on import the same upward migrations run as for domain objects,
  so an old archive stays readable.
* **Incremental** on `updated_at`/`seq` against a parent archive; deletions travel as tombstones,
  or deleted objects would come back on restore. Content-addressed media are not re-transferred: a
  medium lives in the archive of the chain that first referenced it, and a restore searches the
  chain newest first.
* **Encrypted** (§4) before it leaves the process. The manifest is the one member never encrypted,
  so §8.1 can list archives with only the target credentials — hence no user content in it. It is
  not signed; `checksums.txt` catches corruption, the authenticated cipher an attacker.
* **Verifiable in place:** `POST /backups/{id}:verify` checks an archive at the target without
  restoring it. An archive without `checksums.txt` is a run that died or is still running, not a
  damaged one (§8.1).
* **Streamed, not staged**, so memory and disk stay flat however large the holding; a resumed run
  finds what is already at the target with one `List`.

**Golden archives**, one per archive format version, are committed under `test/backup/golden/` and
imported by BK-4; one is added at a major release
([versioning-release.md](./versioning-release.md) §1, §7).

---

## 4. Encryption

Backups sit on somebody else's storage. Therefore:

* **Client-side encryption is the standard**: AES-256-GCM, with a key **derived from the
  installation's master key with HKDF-SHA256, bound to the target** — two targets never share a key,
  and nobody remembers a second secret. The key is **not** in the archive; without the master key
  the backup is unreadable. Server-side encryption at the target may be added; it replaces nothing.
* **A passphrase is not available.** `encryption_passphrase` on a target and `decryption_passphrase`
  on a restore are refused with `backup.encryption_passphrase_not_available` — a passphrase with no
  effect would leave somebody believing it protects the archive.
* The master key can be rotated; old archives stay readable (the key ID is in the manifest).
* An unencrypted target (`encryption_mode: NONE`) requires explicit confirmation
  (`insecure_acknowledged`) and carries the warning `backup.target_unencrypted` for as long as it
  exists.

---

## 5. Schedule and execution

Schedules are RRULE-based — the same mechanism as recurring tasks:

```json
{
  "target_id": "…",
  "scope": { "kind": "TENANT", "id": "…" },
  "schedule": "FREQ=DAILY;BYHOUR=3;BYMINUTE=0",
  "timezone": "Europe/Berlin",
  "mode": "INCREMENTAL", "full_every": "FREQ=WEEKLY;BYDAY=SU",
  "retention": { "keep_last": 7, "keep_daily": 14, "keep_weekly": 8, "keep_monthly": 12, "keep_yearly": 3, "min_keep": 3 },
  "include_media": true, "include_audit": true, "trial_restore": true,
  "notify_on": ["FAILURE", "FIRST_SUCCESS_AFTER_FAILURE"]
}
```

**Schedule rules:**

* **The rule counts from when the schedule was created**, in its own zone — not from "now", which
  would drift a weekly backup to whatever weekday a pod restarted on.
* **`full_every` selects among the schedule's occurrences**; it produces none of its own (the daily
  run on a Sunday is the full one), compared by calendar day in the schedule's zone.
* **Who fires what.** A tenant's schedules are fired by that tenant's own poller, seeded by the write
  that created a schedule ([multi-tenancy.md](./multi-tenancy.md) §2.1). An instance-wide schedule
  belongs to no tenant and is fired by the leader under the installation scope, which reaches only
  rows that have no tenant.

**`trial_restore`** — the job that wrote a `FULL` archive follows it with an `INSPECT` restore of
it: every member read, every checksum verified, every encrypted member decrypted, the difference
report kept on the run, which is then marked verified. A trial that fails **fails the run**
(`backup.trial_restore_failed`, with the reader's code and the member), and `notify_on` covers it.
An incremental is read back with its chain when the next full one is. On by default for a new
schedule; older schedules keep it off until their owner changes it. The trial proves an archive can
be *read*; the quarterly `NEW_TENANT` drill proves a workspace can be *stood up* from it (§10).

**Execution** is an ordinary job (the `worker` role) with progress, cancellation, resumption after
process death, and one run per target at a time. Reads go through a replica or at a throttled rate,
on a bulkhead pool separate from the API path.

* **The lock is the insert** (`… WHERE NOT EXISTS (… status = 'RUNNING' AND id <> …)`): a resumed
  run continues its own row (BK-7); a second run is answered "no", which is not an error.
* **Consistency:** one `REPEATABLE READ` snapshot; media locations are resolved inside it, the bytes
  fetched by checksum after it.
* The run is **detached** from the runner's transaction (it streams for minutes); its completion is
  written in a short transaction of its own, safe because the run is repeatable.

---

## 6. Retention of backups

Separate from the retention of business data ([data-retention.md](./data-retention.md)); neither
reads the other's tables.

1. **The generation principle** (`keep_last`, `keep_daily`, `keep_weekly`, `keep_monthly`, `keep_yearly`).
2. **`min_keep`** as a floor: retention may never leave **no** backup. If a run fails, old archives are not deleted.

Rules:

* **Only Hubtask's own archives expire**, recognised by the manifest and the name prefix
  `hubtask-backup-<tenant>-`; other files at the target are never touched, so a tenant export
  (`hubtask-export-…`) is never pruned. Deletion is audited.
* **What is at the target is read from the manifests, not from the database.** `checksums.txt` is
  removed first, so an interrupted deletion leaves something that reads as unfinished, not as sound.
* **An archive another kept archive needs is kept**: deleting a parent would silently destroy every
  incremental after it.
* **A run with no schedule behind it deletes nothing** — a manual backup made before something risky
  must not delete its neighbours.
* At a target with object lock/WORM, a non-deletable archive is reported as a notice, not retried
  endlessly.

---

## 7. Backup and data protection

An erasure takes effect at once in the primary system, but last week's archive still holds the
data:

* **The deletion journal.** Deletions are recorded in `deletion_journal`; a restore reapplies them
  for the window between the archive and now, so objects deleted in between do not come back. A
  record that *points at* something the journal kept out is kept out with it, so a restore never
  leaves a row referencing an object it deliberately did not create.
* **The retention of the backups is the upper bound on deletion.** For the operator's system backups
  and PITR window it is **35 days** ([data-protection.md](./data-protection.md) §5), and that plan
  keeps no monthly or yearly generation. A tenant's own archive backups are the tenant's to plan; the
  generation defaults of §6 are where they start.
* **A point-in-time restore is the one restore the journal does not cover**: the journal lives in
  the database being rewound. Before traffic is admitted, the erasures completed after the restore
  point are read from the audit export at the backup target — outside the rewound database — and
  re-run (§8.5 step 5).
* `include_audit` is configurable: better evidence against longer persistence of personal metadata.
* No route hands an archive to a caller today; when one exists its download is an audited access
  (`backup.downloaded`, reserved).
* An archive at a target is the operator's responsibility; a third-country target is a transfer
  under GDPR Chapter V, which Hubtask says during target setup, recording the region.

---

## 8. Restore

### 8.1 Browsing the target

`GET /backup-targets/{id}/backups` lists the archives at the target **from the manifests there**,
with no state in the database — a restore works when the database is lost and only the target
credentials exist. It also says whether the run that wrote an archive finished (damaged vs still
being written). The only database access is the target's own row and its sealed credential (the
use case has no run repository). At a shared target the archive's name is the filter, so one tenant
is never told about another's; asking for another tenant's archives outright is refused, not
answered with an empty list.

### 8.2 Modes

| Mode | Effect | Typical occasion |
|---|---|---|
| `INSPECT` | Read the archive, show a content overview and the difference against the current state; changes nothing | "What if" |
| `SELECTIVE` | Pull selected containers/items back into the existing tenant | An accidentally deleted collection |
| `MERGE` | Import the archive, handling existing objects by rule (`skip`, `overwrite`, `duplicate`) | Merging, partial loss |
| `REPLACE_TENANT` | Reset the tenant entirely to the archive state | A serious error, ransomware |
| `NEW_TENANT` | Import the archive as a new tenant | Migration, a test copy, forensics |
| `INSTANCE` | Import a system backup | Total loss — refused, see below |

**Who may use which mode.** A workspace's own screens offer neither `NEW_TENANT` nor `INSTANCE`: one
creates a workspace and the other crosses all of them, and both are the installation operator's. The
API still accepts `NEW_TENANT` from a workspace member holding `STRUCTURE` (open point B-6).
`INSPECT`, `SELECTIVE` and `MERGE` need `STRUCTURE`; the destructive modes need `DELETE_CONTAINER`.
A mode that writes into a living tenant writes only into the caller's own (BK-10).

**Mode rules:**

* **Destructive is `REPLACE_TENANT` and `INSTANCE`** — not `MERGE` with `overwrite`, which replaces
  only the objects the archive names; a replace removes what the archive does *not* name. Only the
  destructive modes ask for the typed workspace name and a step-up (§8.3).
* **`NEW_TENANT`** imports beside the living data — the cheap way to look before a destructive mode.
  The use case mints its tenant identifier; the caller never names it. The copy keeps names and
  calendar UIDs: every unique index is per tenant.
* **`duplicate` applies to content, not to context.** Accounts, media and webhook subscriptions fall
  back to `skip`, and the report says so. What is copied gets a **derived** identity, so a resumed
  restore produces the same identifiers and the copies point at each other.
* **A copy also changes what the schema insists is unique.** Each entity declares the columns a copy
  may not carry unchanged; an integration test compares that with the unique indexes. Today:
  * **A name is suffixed** at the top of the duplicated tree, where it is not already free:
    `Errands (restored 2026-09-24 a1b2c3)` — six characters derived from the run, so restoring the
    same archive twice still lands and a resumed restore produces the same name.
  * **A calendar UID is dropped** (`wi_calendar_uid_uq`): the copy is not the entry the client made.
  * **A tenant-wide custom field definition is not copied** (`skip`): `work_item.custom_fields` is
    keyed by it, so it cannot be suffixed. One inside a collection moves with the collection.
* **`SELECTIVE`'s closure comes from the archive's reference graph**; containers need a pass of
  their own, because a sub-collection can be written before its hub.
* **`INSTANCE` is refused**: no archive this build writes has an instance-wide scope, because system
  backups are the operator's. The scope check answers `backup.restore_instance_is_the_operators`,
  whose message points at §8.5.

### 8.3 The procedure

1. **Pre-check:** checksums, schema/product version, decryptability, scope, estimated duration.
2. **Dry run with a report:** new/overwritten/skipped objects, conflicts. `dry_run` is true by
   default. A client offers the real run only after the rehearsal's report has been shown, and
   composes it from the rehearsed request (`dry_run: false`), so what was reviewed is what runs.
3. **Confirmation for destructive modes:** the tenant name typed (`confirmation`) **and** a step-up
   ([security.md](./security.md) §5): a fresh re-authentication on the current session, valid for
   `HUBTASK_STEP_UP_WINDOW`, sent as `step_up_token` and consumed by the one restore it is presented
   to. Without it: `403 auth.step_up_required` with the accepted methods. An unwired verifier
   refuses rather than permits.
4. **A safety copy of the current state** before a destructive mode (`create_safety_backup`, on by
   default). Nowhere to write it **stops the restore**. The copy's identifier is recorded on the run
   *before* the destructive mode runs, so the way back is findable even if the run fails.
5. **Execution as a job with progress**; a cancellation rolls back at most the batch in flight. Each
   batch commits its rows and the progress marker in one transaction, so a replacement worker
   continues rather than re-deciding. One restore at a time per workspace
   (`backup.restore_in_progress`). A child arriving before its parent is deferred and written in
   rounds once the entity's stream is exhausted; what a round that settled nothing leaves without a
   parent is withheld as `orphaned`, and the progress marker stops before the first deferred row
   until it settles.
6. **Follow-up:** apply the deletion journal (§7); the search index needs no pass (a trigger indexes
   each row as it lands); fire **no** automation for the period (§8.4); write the report to the audit.
   A restore into an existing workspace (`REPLACE_TENANT`, `MERGE`, `SELECTIVE`) advances the
   workspace's `sync_epoch` in the transaction that records its success, so every older device
   cursor answers `sync.cursor_too_old` and the device resynchronises
   ([offline-sync.md](./offline-sync.md) §3.1). A dry run and a `NEW_TENANT` restore advance nothing.

**A destructive restore ends sign-ins.** The archive carries no session or token tables (§8.4), so
the credential that started a `REPLACE_TENANT` is refused part-way through following its own job;
read `restore_run` for the result instead.

### 8.4 What deliberately does *not* happen during a restore

* **No automation rules fire.** Restored changes produce events with `replay: true`; the dispatcher
  withholds them from the rule engine, so a subscriber added later is never handed a replay.
* **No past reminders are caught up**; they become `LAPSED` (nobody cancelled them).
* **No webhooks are re-delivered**; the archive's outbox is not imported.
* **No tokens, sessions or sign-in providers are restored** — none of `identity_provider`,
  `auth_pending` or the session and token tables is in the archive. Users sign in again; PATs are
  recreated. This is shown before the restore.
* **The audit trail is in the archive and not written back.** Inserting last month's entries into a
  live hash chain is a rewrite, not a restore ([audit.md](./audit.md) §3). `include_audit` keeps the
  evidence readable where it was written down.
* **Retention rules are not in the archive**: `EXPORT_THEN_DELETE` names an egress a restore does not
  recreate.

### 8.5 The operator procedure: point-in-time recovery

The **operator's** recovery of the whole installation from the continuous WAL archive (§1's system
backup, [ADR-0046](../adr/ADR-0046-production-on-a-platform-namespace.md)); the alerts that watch
the archive are in [RB-A12](../../deploy/observability/runbooks/RB-A12-backup-stale.md).

**Hubtask does not perform this restore** — an application cannot restore the database it needs to
run. CloudNativePG performs it from the archive the `Cluster` writes
([`k8s/templates/cnpg-cluster.yaml`](../../k8s/templates/cnpg-cluster.yaml)), with the platform's
volume snapshots as a second net; Hubtask contributes the drill and the checks. **A person alone
can carry out the procedure.**

#### The procedure

1. **Decide the moment** before the damage, from the audit trail or the incident timeline;
   everything after it is discarded. Nobody can automate this step.
2. **Stop writing** — scale the workloads to zero, or the database ends up with two histories.
3. **Bootstrap a new cluster from the archive**: `bootstrap.recovery` names the external cluster,
   `recoveryTarget.targetTime` the moment. Always a *new* cluster, never over the live one — the
   archive is the only copy of the history being replayed.
4. **Check what came back before admitting traffic**, with the drill's checks (`cmd/restore-drill`
   against a target time): the markers, the schema version, index and constraint validity, row
   level security forced on every tenant table, the application role still unable to bypass it.
5. **Re-apply the erasures the rewind undid**: read those completed after the recovery point from
   the audit export at the backup target — outside the rewound database — and re-run them before
   traffic is admitted (§7).
6. **Point the application at the recovered cluster** and scale back up; forward-only migrations
   take the recovered schema the rest of the way.
7. **Write down what happened** for the incident record. RPO and RTO figures stay internal and are
   not published in this repository.

#### What proves it in advance

**RT-9, per release (a hook) and weekly (a `CronJob`):** `hubtask-restore-drill` writes two marker
rows with a recorded moment between them, recovers a temporary cluster to that moment, expects the
first marker and not the second, runs step 4's checks, measures RPO and RTO, and removes the
temporary cluster whatever happened. A pass updates the record A-20 watches (§10). A failed drill
does **not** fail the release; the record keeps the previous success. **In CI**, `make gate-pitr`
runs the same on a kind cluster with the CloudNativePG operator and an S3-compatible store — it
proves the path, not the size of the numbers.

### 8.6 The minimal path: a dump, and what it does not give

On the two-container Compose stack the system backup is a dump, with the media beside it (the
database references objects it does not contain):

```bash
docker compose exec -T db pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc \
  > "hubtask-$(date -u +%Y%m%dT%H%M%SZ).dump"
docker compose cp app:/var/lib/hubtask/media ./media-backup
# Putting it back, into a database that is empty:
docker compose exec -T db pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists \
  < hubtask-20260907T020000Z.dump
```

**It gives** a consistent snapshot of the whole installation, restorable onto the same PostgreSQL
major version. **It does not give** what §8.5 gives:

* **No point in time but the ones you took** — nightly means up to a day lost; the RPO of ≤ 5 minutes
  in [observability-reliability.md §2](./observability-reliability.md#2-service-level-objectives)
  needs continuous WAL archiving.
* **No protection against a deletion you copy** — a dump written over the previous one is a backup
  of the damage. Object lock on the target prevents that (B-3).
* **Nothing has restored it.** The drill metric stays absent, so **A-20 never fires and never
  reassures**. After a restore you checked yourself, write the record:

  ```bash
  date -u +%s > /var/lib/hubtask/restore-drill/last_success_unix
  # compose.yaml: HUBTASK_RESTORE_DRILL_RECORD_FILE: /var/lib/hubtask/restore-drill/last_success_unix
  ```

* **Tenant archives are a different promise**: they work in the Compose stack, but hold one
  workspace's content, not the installation's database.

For one person's own installation, a nightly dump to a second machine plus the tenant archives is
defensible — a day of writes and an untested archive short of §8.5.

---

## 9. Import and export of existing systems

**Importers write archive records, and the restore applies them** — one ingestion path.
`POST /imports` names a file uploaded with `usage: IMPORT` and the hub the collections land under; a
converter per kind (`infrastructure/importer`) turns it into `archive.Record`s in memory, and the
applier lands them in `MERGE` mode with `skip`.

| Kind | Source |
|---|---|
| CSV | A CSV file. The collection is named after the file (`errands.csv` → *errands*); a `collection` column overrides it per row |
| Trello | A board export |
| Google Tasks | Google Takeout's `Tasks.json` |
| Microsoft To Do | The Graph API's JSON, **fetched by the person** (`hubctl import todo` shows the requests) — a live connection would need a token this product would have to hold. Windows zone names are mapped by CLDR's `windowsZones`; a row with an unknown zone is refused, not guessed |

Rules:

* **Identities are derived** from the hub and the source's own identity (a CSV's digest, a board's
  identifier), so importing the same source into the same hub twice is a no-op.
* **A different file whose collection meets a name the hub already holds** is refused on the run with
  `imports.collection_exists` and lands nothing; the applier answers a unique index as a conflict
  (`containers.name_taken`, `backup.row_conflicts`), never as a database error the queue would retry.
* The run (`import_run`) carries the report and the refused row numbers; the file is deleted when
  the job ends; the workspace's `sync_epoch` advances as after a `MERGE` restore, because the rows
  land without change log entries.

**Exports are archives too.** A tenant export (`POST /admin/tenants/{id}:export`,
[tenant-export.md](./tenant-export.md)) and a data subject export
([data-protection.md](./data-protection.md) §4, prefix `hubtask-dsr-`) are Hubtask archives written
unencrypted to a backup target — restorable, and no second format exists.

---

## 10. Self-diagnosis and alerts

A warning is named after what it describes: `backup.target_*` is one **a target carries about
itself**, in the resource's `warnings` array; `config.backup_*` is about **the installation**.

| Signal | Meaning |
|---|---|
| `backup.target_unencrypted` (on the resource) | This target stores archives unencrypted |
| `backup.target_plaintext_protocol` (on the resource) | This target is reached over a connection anybody on the wire can read — judged by the configured scheme for every URL-addressed kind, and by the name for `ftp` |
| `config.backup_not_configured` | No target configured |
| `config.backup_unencrypted` | A target without encryption |
| `config.backup_single_target` | Only one target — a pointer to 3-2-1 |
| A-12 | No successful backup in 24 hours, **per target** — a `max()` across targets would let a healthy target hide a broken one. `hubtask_backup_last_success_timestamp_seconds` is **absent**, not zero, for a target that never had one ([RB-A12](../../deploy/observability/runbooks/RB-A12-backup-stale.md)) |
| A-20 | The system restore drill (§8.5) is older than 90 days — a ticket, not a page. `hubtask_restore_drill_last_success_timestamp_seconds` is read from `HUBTASK_RESTORE_DRILL_RECORD_FILE`, absent where nothing recorded a drill; the tenant-level `NEW_TENANT` trial does not write it. A-20 has no `absent()` rule, because an installation without a drill (Compose) records nothing; where the chart owns the database, `HubtaskRestoreDrillNeverRan` (`prometheus-rules-pitr.yaml`) fires on the absence ([RB-A20](../../deploy/observability/runbooks/RB-A20-restore-drill-stale.md)) |

The three `config.backup_*` codes are in the catalogue and **emitted by nothing yet**: `/meta/health`
is process-wide and a target is a tenant row behind RLS, so they wait for a workspace-facing health
check.

**The tenant-level drill is the client:**

```bash
hubctl backup run --target "$TARGET" --follow --wait 30m
hubctl backup verify "$RUN" --follow
hubctl restore inspect --target "$TARGET" --archive "$ARCHIVE"
hubctl restore run --target "$TARGET" --archive "$ARCHIVE" --mode NEW_TENANT --apply
```

`scripts/hubctl-e2e.sh` runs exactly that against the reference Compose stack on every pull request
and compares the restored and the source workspace's entry counts.

---

## 11. Evidence

| Test | Contents |
|---|---|
| BK-1 | A round trip per adapter (local, s3, sftp, webdav) against a test container: back up, list, verify, restore |
| BK-2 | An encrypted archive is unreadable without the key; with a rotated key, the old archive stays readable |
| BK-3 | An incremental chain over 10 runs including deletions reproduces the source state exactly |
| BK-4 | An archive from an older schema version imports correctly (golden archives, one per format version) |
| BK-5 | A restore triggers no automation, sends no webhooks or emails, and restores no tokens |
| BK-6 | The deletion journal prevents deleted objects from returning |
| BK-7 | Process death during a backup and during a restore: resumption without duplicates |
| BK-8 | Retention deletes according to the generation plan, `min_keep` is never undercut, and other files at the target stay untouched |
| BK-9 | A target configuration pointing at an internal address is blocked by `GuardedClient` unless explicitly released |
| BK-10 | Cross-tenant: tenant A cannot list, verify, or restore an archive belonging to B |
| BK-11 | A `FULL` run with `trial_restore` reads its archive back and carries the report; one damaged before the trial fails with `backup.trial_restore_failed`; an older schedule keeps it off |

---

## 12. Open points

| # | Point | Needed by |
|---|---|---|
| B-1 | Whether `rclone` goes into the image (size, and its GPL-3.0 licence alongside Apache-2.0) | Before an `rclone` adapter |
| B-6 | `NEW_TENANT` is the operator's and the web app does not offer it, but `StartRestore` accepts it from any member holding `STRUCTURE`. Either the API moves it behind `admin:tenants` (and the drill of §10 with it) or the rule changes | Before `1.0.0` |

**Object lock (B-3).** Required for the system backup target, recommended for a tenant's own. The
credential that writes backups must not be able to delete them or shorten their retention. For the
system backup the lock retention **equals** 35 days: longer and the plan's own cleanup fails against
the lock, shorter and [data-protection.md](./data-protection.md) §5 is not kept by the storage. A
tenant enabling it owes itself the same arithmetic.

Closed points cited elsewhere: B-2 (system backups are left to the operator) is §1 and §8.5; B-3 is
the paragraph above; B-4 (the trial restore is an `INSPECT` of every `FULL` archive) is §5; B-5 (what
a restore owes connected devices: the `sync_epoch`) is §8.3 step 6.
