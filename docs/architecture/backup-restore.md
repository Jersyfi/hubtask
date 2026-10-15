# Backup and Restore

Backup is a **feature of the application**: targets, schedules and retention are configurable, and
restores run from the archives listed at a target
([ADR-0019](../adr/ADR-0019-backup-targets.md)).

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
(`infrastructure/backupstorage`); none is preferred. A kind with no adapter is refused with
`backup.kind_unsupported`; an adapter ships when it passes the conformance suite (BK-1).

| Adapter | Built | Protocol / notes |
|---|---|---|
| `local` | yes | A directory in the backup volume (`HUBTASK_BACKUP_LOCAL_PATH`, the self-hosting default). A target's path is **relative** to that volume and cannot leave it: whoever configures a target administers the instance, not the machine |
| `s3` | yes | Any S3-compatible endpoint; server-side encryption and object lock usable. Uploaded in parts, so the process holds one part, not an archive |
| `sftp` | yes | An in-house SFTP v3 client over `golang.org/x/crypto/ssh` ([security.md](./security.md) §11), password or key. The host key is **configuration** — the server's public key or its SHA-256 fingerprint; a target naming neither is refused. No trust on first use, no way to switch the check off |
| `webdav` | yes | Nextcloud, ownCloud, generic WebDAV. Listed by recursing `PROPFIND` at depth one (Apache refuses infinite depth by default) |

Not built: `ftps`; `ftp` (only with explicit confirmation, a warning and an audit entry); `smb`;
`azure_blob`, `gcs`; `rclone` (B-1); `http_put`. Several targets in parallel (3-2-1) each have their
own schedule and retention.

**The target configuration is a narrow exception to the SSRF rule** of [security.md](./security.md):

* **Who may create a target:** the owner's right in the role matrix; in single mode the owner *is*
  the instance administrator. In provider operation a tenant may have its own targets only when the
  operator sets `HUBTASK_BACKUP_TENANT_TARGETS=true` (off by default), with an egress allowlist on
  top; otherwise `backup.tenant_targets_disabled`.
* **Every call to a target** goes through `GuardedClient` (SFTP through its resolver and dial-time
  control): metadata endpoints, RFC 1918 ranges and loopback are refused unless the operator sets
  `HUBTASK_HTTP_ALLOW_PRIVATE_NETWORKS`, and no redirect is followed (BK-9).
* **Creating or changing a target is audited** (`backup.target_changed`: kind, configuration,
  encryption mode, never the credential). **Removing one** touches nothing at the target
  (`backup.target_removed`).
* **Credentials are sealed** ([security.md](./security.md) §8), bound to their row, and read back by
  exactly one repository method; no statement feeding a response selects the column.

---

## 3. The archive format

The container, manifest, naming, commit point, media addressing and verification are
[tenant-export.md](./tenant-export.md) §1, §3–§8. What a backup adds:

* **JSON Lines, not an SQL dump**: an import runs the same upward migrations as for domain objects,
  so an old archive stays readable.
* **Incremental** on `updated_at`/`seq` against a parent archive; deletions travel as tombstones, or
  deleted objects would come back. A medium lives in the archive of the chain that first referenced
  it; a restore searches the chain newest first.
* **Encrypted** (§4) before it leaves the process, all but the manifest. The manifest is not signed:
  `checksums.txt` catches corruption, the authenticated cipher an attacker.
* **Verifiable in place:** `POST /backups/{id}:verify` checks an archive at the target without
  restoring it.
* **Streamed, not staged**: memory and disk stay flat; a resumed run finds what is already at the
  target with one `List`.

**Golden archives**, one per format version, live under `test/backup/golden/` and are imported by
BK-4; one is added at a major release ([versioning-release.md](./versioning-release.md) §1, §7).

---

## 4. Encryption

* **Client-side encryption is the standard**: AES-256-GCM under a key **derived from the
  installation's master key with HKDF-SHA256, bound by a random salt** each archive is given when
  it is written and its manifest carries (`encryption.archive_salt`). No two archives share a key,
  so no two targets do; nobody remembers a second secret; and a target re-created after the
  database is lost still opens the archives at its location: the salt and the master key's ID are
  read from the manifest, not from the database. The key is not in the archive. Server-side
  encryption at the target may be added; it replaces nothing. An archive written before archives
  carried a salt keeps its key bound to the target that wrote it, and opens only through that
  target's row.
* **What to keep for a loss of the database**: the master key ring (`HUBTASK_ENCRYPTION_KEYS`, every
  key with its ID as it was when the archives were written) and the target's credentials. Given
  both, a new installation lists the archives at the target and restores one as a new workspace
  (§8.2). Without the key an archive names, a restore is refused before anything is read
  (`backup.archive_key_unknown`, naming the key).
* **A passphrase is not available**: `encryption_passphrase` on a target and `decryption_passphrase`
  on a restore are refused (`backup.encryption_passphrase_not_available`) rather than ignored.
* The master key can be rotated; old archives stay readable (the key ID is in the manifest). An
  archive is never rewritten, so a re-seal cannot move it: the key census counts the archives still
  kept under each key ([security.md](./security.md) §8.1), and a key stays in the ring until the
  last of them has expired.
* An unencrypted target (`encryption_mode: NONE`) requires `insecure_acknowledged` and carries the
  warning `backup.target_unencrypted` for as long as it exists.

---

## 5. Schedule and execution

Schedules are RRULE-based, like recurring tasks:

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
* **`full_every` selects among the schedule's occurrences** by calendar day in the schedule's zone
  (the daily run on a Sunday is the full one); it produces none of its own.
* A tenant's schedules are fired by that tenant's own poller, seeded by the write that created one
  ([multi-tenancy.md](./multi-tenancy.md) §2.1); an instance-wide schedule by the leader under the
  installation scope, which reaches only rows without a tenant.

**`trial_restore`** — the job that wrote a `FULL` archive follows it with an `INSPECT` restore:
every member read, checksum verified and decrypted, the difference report kept on the run, which is
marked verified. A failed trial **fails the run** (`backup.trial_restore_failed`, with the reader's
code and the member), and `notify_on` covers it. An incremental is read back with its chain at the
next full one. On by default for a new schedule; older schedules keep it off until changed. The
trial proves an archive can be *read*; the quarterly `NEW_TENANT` drill that a workspace can be
*stood up* from it (§10).

**Execution** is an ordinary job (the `worker` role) with progress, cancellation, resumption after
process death, and one run per target at a time, reading through a replica or throttled, on a
bulkhead pool apart from the API path.

* **The lock is the insert** (`… WHERE NOT EXISTS (… status = 'RUNNING' AND id <> …)`): a resumed
  run continues its own row (BK-7); a second run is answered "no", not an error.
* **Consistency:** one `REPEATABLE READ` snapshot resolves the media locations; the bytes are
  fetched by checksum after it.
* The run is **detached** from the runner's transaction; its completion is written in a short
  transaction of its own, safe because the run is repeatable.

---

## 6. Retention of backups

Separate from business-data retention ([data-retention.md](./data-retention.md)); neither reads
the other's tables.

1. **The generation principle** (`keep_last`, `keep_daily`, `keep_weekly`, `keep_monthly`, `keep_yearly`).
2. **`min_keep`** as a floor: retention may never leave **no** backup. If a run fails, old archives are not deleted.

Rules:

* **Only Hubtask's own archives expire**, recognised by the manifest and the prefix
  `hubtask-backup-<tenant>-`; nothing else at the target is touched (a tenant export is never
  pruned). Deletion is audited.
* **What is at the target is read from the manifests, not the database.** `checksums.txt` is removed
  first, so an interrupted deletion reads as unfinished, not as sound.
* **An archive another kept archive needs is kept** — a parent's loss would destroy every
  incremental after it.
* **A run with no schedule behind it deletes nothing**: a manual backup before something risky must
  not delete its neighbours.
* Under object lock/WORM, a non-deletable archive is reported as a notice, not retried endlessly.

---

## 7. Backup and data protection

An erasure takes effect at once in the primary system, but last week's archive still holds the
data:

* **The deletion journal.** Deletions are recorded in `deletion_journal`; a restore reapplies those
  between the archive and now, and keeps out with them every record whose identity rests on what
  they kept out (a join row, an attachment), so no restored row references an object deliberately
  not created. A reference the record merely uses and may lose — an entry's cover image — is
  cleared instead: the entry comes back without its cover.
* **Backup retention bounds deletion**: 35 days for the operator's system backups and PITR window
  ([data-protection.md](./data-protection.md) §5). A tenant's own archive backups are the tenant's to
  plan, starting from §6's generation defaults.
* **A point-in-time restore is the one restore the journal does not cover** — the journal lives in
  the rewound database; §8.5 step 5 places the legal holds again and re-applies the erasures.
* `include_audit` weighs evidence against longer persistence of personal metadata.
* No route hands an archive to a caller; one that does will audit the download (`backup.downloaded`,
  reserved).
* An archive at a target is the operator's responsibility. A third-country target is a transfer
  under GDPR Chapter V; target setup says so and records the region.

---

## 8. Restore

### 8.1 Browsing the target

`GET /backup-targets/{id}/backups` lists the archives **from the manifests at the target**, with no
state in the database, so a restore works with only the target credentials; it also tells a damaged
archive from one still being written. The only database access is the target's own row and sealed
credential. At a shared target the archive's name is the filter, so no tenant learns of another's;
asking for another tenant's archives outright is refused, not answered with an empty list. The
operator's listing — a credential carrying `admin:tenants` — also shows, for a `NEW_TENANT`
restore, archives whose manifest names a workspace this installation does not hold (§8.2); never one
of a workspace it does. Whether a workspace exists is read in that workspace's own scope, one read
per workspace named.

### 8.2 Modes

| Mode | Effect | Typical occasion |
|---|---|---|
| `INSPECT` | Read the archive, show a content overview and the difference against the current state; changes nothing | "What if" |
| `SELECTIVE` | Pull selected containers/items back into the existing tenant | An accidentally deleted collection |
| `MERGE` | Import the archive, handling existing objects by rule (`skip`, `overwrite`, `duplicate`) | Merging, partial loss |
| `REPLACE_TENANT` | Reset the tenant entirely to the archive state | A serious error, ransomware |
| `NEW_TENANT` | Import the archive as a new tenant | Migration, a test copy, forensics |
| `INSTANCE` | Import a system backup | Total loss — refused, see below |

**Who may use which mode.** `NEW_TENANT` creates a workspace and `INSTANCE` crosses all of them, so
both are the installation operator's and a workspace's screens offer neither: the API asks for the
`admin:tenants` scope on top of the role below (`access.insufficient_scope` without it), and the
authentication withholds that scope from anybody the operator register does not name. The role is
still asked because the archive is read through the caller's own workspace's target. `INSPECT`,
`SELECTIVE` and `MERGE` need `STRUCTURE`, as does `NEW_TENANT`; the destructive modes
`DELETE_CONTAINER`. A mode that writes into a living tenant writes only into the caller's own
(BK-10).

**Mode rules:**

* **Destructive is `REPLACE_TENANT` and `INSTANCE`** — not `MERGE` with `overwrite`, which replaces
  only the objects the archive names; a replace also removes what it does *not* name. Only the
  destructive modes ask for the typed workspace name and a step-up (§8.3).
* **A legal hold stops a destructive mode.** `REPLACE_TENANT` is refused while any hold in the
  workspace is in force; one that runs keeps the workspace's own hold records and never takes the
  archive's, so a hold is neither dropped nor revived
  ([data-protection.md](./data-protection.md) §5). `MERGE` and `SELECTIVE` leave the hold records
  alone under every conflict rule; only `NEW_TENANT` writes the archive's, so a new workspace keeps
  the obligations it arrives with; the report counts the holds it did not take as
  `legal_hold_kept`. The refusal is asked when the restore is requested and again by the job in the
  transaction that would empty the workspace; a rehearsal stays possible. A restore into a living
  workspace also leaves a `RESTRICTED` account as it is: the replace keeps its row, and an
  overwrite passes it by ([data-retention.md](./data-retention.md) §4 item 2).
* **`NEW_TENANT`** imports beside the living data — the cheap look before a destructive mode. The use
  case mints the tenant identifier; the caller never names it. The copy keeps names and calendar
  UIDs: every unique index is per tenant.
* **An archive is restored only from its own workspace**, in every mode, with one exception: the
  operator's `NEW_TENANT` accepts an archive whose manifest names a workspace that does not exist on
  this installation — another installation's export, the provider-migration path
  ([tenant-export.md](./tenant-export.md) §10). An archive of another workspace that does exist here
  stays refused, so BK-10 holds unchanged between this installation's workspaces; both refusals
  answer `backup.restore_archive_scope_mismatch`. An encrypted archive from another installation
  stays out of reach (§4, #1075): it is refused before anything is read, with
  `backup.restore_foreign_archive_encrypted`, not left to fail as a decryption error.
* **`duplicate` applies to content, not to context.** Accounts, media and webhook subscriptions fall
  back to `skip`, and the report says so. A copy gets a **derived** identity, so a resumed restore
  produces the same identifiers and the copies point at each other.
* **A copy also changes what the schema insists is unique.** Each entity declares the columns a copy
  may not carry unchanged; an integration test compares that with the unique indexes. Today:
  * **A name is suffixed** at the top of the duplicated tree where it is not free:
    `Errands (restored 2026-09-24 a1b2c3)`, six characters derived from the run, so the same archive
    restored twice still lands and a resumed restore produces the same name.
  * **A calendar UID is dropped** (`wi_calendar_uid_uq`): the copy is not the entry the client made.
  * **A tenant-wide custom field definition is not copied** (`skip`): `work_item.custom_fields` is
    keyed by it. One inside a collection moves with the collection.
* **`SELECTIVE`'s closure comes from the archive's reference graph**; containers need a pass of
  their own, because a sub-collection can be written before its hub.
* **`INSTANCE` is refused**: no archive this build writes has an instance-wide scope. The scope
  check answers `backup.restore_instance_is_the_operators`, whose message points at §8.5.

### 8.3 The procedure

1. **Pre-check:** checksums, schema/product version, decryptability, scope, estimated duration.
2. **Dry run with a report:** new/overwritten/skipped objects, conflicts. `dry_run` is true by
   default. A client offers the real run only after showing the rehearsal's report, and composes it
   from the rehearsed request (`dry_run: false`), so what was reviewed is what runs.
3. **Confirmation for destructive modes:** the tenant name typed (`confirmation`) **and** a step-up
   ([identity.md](./identity.md) §16) sent as `step_up_token`. An unwired verifier refuses rather
   than permits.
4. **A safety copy of the current state** before a destructive mode (`create_safety_backup`, on by
   default). Nowhere to write it **stops the restore**. Its identifier is recorded on the run *before*
   the destructive mode runs, so the way back is findable even if the run fails.
5. **Execution as a job with progress**; a cancellation rolls back at most the batch in flight. Each
   batch commits its rows and the progress marker in one transaction, so a replacement worker
   continues rather than re-deciding. One restore at a time per workspace
   (`backup.restore_in_progress`). A child arriving before its parent is deferred and written in
   rounds once the entity's stream is exhausted; what a round that settled nothing leaves without a
   parent is withheld as `orphaned`, and the progress marker stops before the first deferred row
   until it settles.
6. **Follow-up:** apply the deletion journal (§7); the search index needs no pass (a trigger indexes
   each row as it lands); fire **no** automation (§8.4); write the report to the audit. A restore
   into an existing workspace (`REPLACE_TENANT`, `MERGE`, `SELECTIVE`) advances its `sync_epoch` in
   the transaction recording its success, so devices resynchronise
   ([offline-sync.md](./offline-sync.md) §3.1); a dry run and a `NEW_TENANT` restore advance nothing.

**A destructive restore ends sign-ins.** The archive carries no session or token tables (§8.4), so
the credential that started a `REPLACE_TENANT` is refused part-way through following its own job;
read `restore_run` for the result instead.

### 8.4 What deliberately does *not* happen during a restore

* **No automation rules fire.** Restored changes produce events with `replay: true`; the dispatcher
  withholds them from the rule engine, and a subscriber added later is never handed a replay.
* **No past reminders are caught up**; they become `LAPSED` (nobody cancelled them).
* **No webhooks are re-delivered**; the archive's outbox is not imported.
* **No tokens, sessions or sign-in providers are restored** — `identity_provider`, `auth_pending`
  and the session and token tables are not in the archive. Users sign in again; PATs are recreated.
  This is shown before the restore.
* **The audit trail is in the archive and not written back**: inserting last month's entries into a
  live hash chain is a rewrite, not a restore ([audit.md](./audit.md) §3).
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
   `recoveryTarget.targetTime` the moment. Never over the live one — the archive is the only copy of
   the history being replayed.
4. **Check what came back before admitting traffic**, with the drill's checks (`cmd/restore-drill`
   against a target time): the markers, the schema version, index and constraint validity, row
   level security forced on every tenant table, the application role still unable to bypass it.
5. **Re-place the legal holds, then re-apply the erasures the rewind undid**: recover a second,
   temporary cluster to the end of the archive — the moment of the loss, as `cmd/restore-drill` does
   for RT-9 — and place again every `legal_hold` row the recovered database lacks, with its original
   id, scope, reason and placer, each recorded as `lifecycle.hold_replaced`. Where the end of the
   archive is unusable, the source is the newest workspace archive that includes the audit trail,
   read for `lifecycle.hold_placed`, and the gap is reported per workspace. Then the data subject
   erasures completed after the recovery point are carried out again from the same temporary
   cluster, both before traffic is admitted (§7). The holds go first, so no re-run erasure removes
   what one of them keeps. The other removals of the rewound period (a trash emptied, a retention
   pass, an operator's deletion) are not re-run: the recovery point is chosen before the damage, and
   retention deletes again on its next pass. A hold released after the recovery
   point stays in force — the safe side — and its owner is told to release it again
   ([data-protection.md](./data-protection.md) §5).
6. **Point the application at the recovered cluster** and scale back up; forward-only migrations
   take the recovered schema the rest of the way.
7. **Write down what happened** for the incident record. RPO and RTO figures stay internal.

#### Step 5's holds, by hand

The holds are placed through the application, not into the database: each one keeps its trail
entry and its workspace's transaction (rule 3). So for step 5 the application already runs against
the recovered cluster — step 6's pointing, with the ingress still closed — and the operator works
with `hubctl` and a credential that holds `admin:tenants`.

1. **Recover the temporary cluster** like step 3, from the same archive and under another name, with
   no `recoveryTarget`: CloudNativePG then replays to the end of the archive. It is read only; it is
   removed after step 5.
2. **Per workspace**, read its holds there and place the missing ones in the recovered cluster:

   ```bash
   export PGPASSWORD=…    # the temporary cluster's app role; never in the connection string
   hubctl admin tenant replace-holds <workspace> --recovery-point <step 1's moment> \
     --from-cluster "postgres://hubtask_app@<temporary>-rw.<namespace>.svc:5432/<database>"
   ```

   `hubctl` reads through a read-only session under the workspace's own scope, and its statement
   names the workspace, so even the cluster's superuser reads one workspace only. The answer lists
   each hold: `PLACED` or already `PRESENT`, whether what it names exists in the recovered
   workspace, and whether it was released in the rewound period — that owner is told to release it
   again. A second run changes nothing.
3. **Where the end of the archive is unusable**, the source is the workspace's newest archive that
   includes the audit trail (`hubctl backup ls --target <target>`), read by the server, which holds
   the archive's key:

   ```bash
   hubctl admin tenant replace-holds <workspace> --recovery-point <step 1's moment> \
     --from-archive <archive path> --target <target>
   ```

   It places the holds the archive's copy of the workspace carries and those its trail records as
   placed or placed again, and prints, per workspace, when the archive was taken — a hold placed
   between then and the loss is not in it, and goes into the incident record as the gap — whether
   the archive included the trail, and how many records named too little to place.

The workspaces are those the recovered installation lists (`hubctl admin tenant ls`); a workspace
created after the recovery point is not among them, and its holds went with it.

#### Step 5's erasures, by hand

After the holds, per workspace, from the same temporary cluster and with a credential that holds
`admin:tenants` and can prove itself afresh (the operation deletes for good, so it takes a step-up,
[identity.md](./identity.md) §16.3):

```bash
export PGPASSWORD=…    # the temporary cluster's app role
hubctl admin tenant reapply-erasures <workspace> --recovery-point <step 1's moment> \
  --from-cluster "postgres://hubtask_app@<temporary>-rw.<namespace>.svc:5432/<database>"
```

`hubctl` reads the workspace's erasure cases completed after the recovery point, under the
workspace's own scope and with the workspace named in its statement. A full deletion's case no
longer names the person once the account went. The reader takes the one account the erasure
removed at the moment of the case's `dsr.erased` entry. Where none or several match, it sends
nothing and reports the case (`SUBJECT_UNRESOLVED`, `SUBJECT_AMBIGUOUS`). The operator then names
the subject from the incident record and sends the case with `--from <file>`.

The server believes the cases only as far as the recovered workspace cannot contradict them:

- A case it holds must be the same erasure (kind, mode, subject), still open.
- A case it lacks must have been received after the recovery point.
- Every case must have been completed between the recovery point and now.

Anything else is answered `CASE_MISMATCH` and left alone.

Each erasure runs as an erasure does, under the holds in force. A case the workspace lacks is
written again with its own identifier. Each case is completed at its own moment, and
`dsr.erasure_reapplied` goes into the trail. The answer lists each case:

| Outcome | Meaning |
|---|---|
| `REAPPLIED` | The erasure was carried out again. |
| `ALREADY_COMPLETED` | The case was already completed. A second run changes nothing, and the rewound job of a case that was open finishing first is the same answer. |
| `SUBJECT_ABSENT` | There is nothing to erase: the account was created after the point, or the case was about an address. |
| `ERASURE_BLOCKED` | The erasure itself refused, with its code. For example, an automation still runs as the person because its re-pointing was rewound too. The case stays open for its owner. |
| `CASE_REJECTED` | The recovered workspace holds the case as rejected. A person decides. |

**Where the end of the archive is unusable**, the erasures cannot be carried out again from an
archive. An archive carries neither the cases, nor the pseudonyms, nor the deletion journal, and its
trail's `dsr.erased` names the case but never the person. The workspace's administrator lists those
entries after the recovery point, and records and carries out the cases again. The operator notes
the workspace in the incident record.

#### What proves it in advance

**RT-9, per release (a hook) and weekly (a `CronJob`):** `hubtask-restore-drill` writes two marker
rows with a recorded moment between them, recovers a temporary cluster to that moment, expects the
first marker and not the second, runs step 4's checks, measures RPO and RTO, and removes the
temporary cluster whatever happened. A pass updates the record A-20 watches (§10); a failed drill
does **not** fail the release. **In CI**, `make gate-pitr` runs the same on a kind cluster with the
CloudNativePG operator and an S3-compatible store — it proves the path, not the numbers.

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

It gives a consistent snapshot of the whole installation, restorable onto the same PostgreSQL major
version — not what §8.5 gives:

* **No point in time but the ones you took**: the RPO of ≤ 5 minutes in
  [observability-reliability.md §2](./observability-reliability.md#2-service-level-objectives) needs
  continuous WAL archiving.
* **No protection against a deletion you copy** — a dump written over the previous one is a backup
  of the damage. Object lock on the target prevents that (B-3).
* **Nothing has restored it.** The drill metric stays absent, so **A-20 never fires and never
  reassures**. After a restore you checked yourself, write the record:

  ```bash
  date -u +%s > /var/lib/hubtask/restore-drill/last_success_unix
  # compose.yaml: HUBTASK_RESTORE_DRILL_RECORD_FILE: /var/lib/hubtask/restore-drill/last_success_unix
  ```

* **Tenant archives are a different promise**: they hold one workspace's content, not the
  installation's database.

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
* **A different file whose collection meets a name the hub already holds** is refused with
  `imports.collection_exists` and lands nothing; the applier answers a unique index as a conflict
  (`containers.name_taken`, `backup.row_conflicts`), never as a database error the queue would retry.
* The run (`import_run`) carries the report and the refused row numbers; the file is deleted when
  the job ends; the `sync_epoch` advances as after a `MERGE` restore, the rows landing without
  change log entries.

**Exports are archives too.** A tenant export ([tenant-export.md](./tenant-export.md)) and a data
subject export ([data-protection.md](./data-protection.md) §4, prefix `hubtask-dsr-`) are Hubtask
archives written unencrypted to a backup target — restorable, and no second format exists. An
installation-wide case's export is one zip, `hubtask-dsr-<case>-<time>.zip`, whose members are the
per-workspace archives unchanged under `<tenant id>/`: a wrapper, not a format.

---

## 10. Self-diagnosis and alerts

`backup.target_*` is a warning **a target carries about itself**, in the resource's `warnings`
array; `config.backup_*` is about **the installation**.

| Signal | Meaning |
|---|---|
| `backup.target_unencrypted` (on the resource) | This target stores archives unencrypted |
| `backup.target_plaintext_protocol` (on the resource) | This target is reached over a connection anybody on the wire can read — judged by the configured scheme for every URL-addressed kind, and by the name for `ftp` |
| `config.backup_not_configured` | No enabled target that an enabled schedule writes to |
| `config.backup_unencrypted` | A target without encryption |
| `config.backup_single_target` | Only one target a schedule writes to — a pointer to 3-2-1 |
| `config.backup_coverage_unchecked` | The targets could not be counted (yet) — never read as "configured" |
| A-12 | No successful backup in 24 hours, **per target**, so a healthy target cannot hide a broken one; the metric is absent, not zero, for a target that never had one ([RB-A12](../../deploy/observability/runbooks/RB-A12-backup-stale.md)) |
| A-20 | The system restore drill (§8.5) is older than 90 days — a ticket, not a page. Read from `HUBTASK_RESTORE_DRILL_RECORD_FILE`; the tenant-level `NEW_TENANT` trial does not write it. No `absent()` rule, because a Compose installation records nothing; where the chart owns the database, `HubtaskRestoreDrillNeverRan` fires on the absence ([RB-A20](../../deploy/observability/runbooks/RB-A20-restore-drill-stale.md)) |

The `config.backup_*` codes are emitted for a **single-tenant installation** only: there its
workspaces' targets are its backups. A target is a workspace's row behind row level security, so the
count is taken per workspace in the background — at start, and again when a report finds it older
than 15 minutes — and counts enabled targets; whether a target has a schedule is not asked. In
provider operation the installation is backed up by its operator at the database level (§1, §8.6),
one workspace's target says nothing about another's, and A-12 watches each target.

**The tenant-level drill is the client:**

```bash
hubctl backup run --target "$TARGET" --follow --wait 30m
hubctl backup verify "$RUN" --follow
hubctl restore inspect --target "$TARGET" --archive "$ARCHIVE"
hubctl restore run --target "$TARGET" --archive "$ARCHIVE" --mode NEW_TENANT --apply
```

The last line is the operator's (§8.2): it runs on a credential that carries `admin:tenants`,
minted by somebody the operator register names — in a private installation with an empty register,
the workspace's owner. `scripts/hubctl-e2e.sh` runs exactly that against the reference Compose stack
on every pull request, checks that the workspace administrator's own credential is refused for the
last line, and compares the restored and the source workspace's entry counts.

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
| BK-10 | Cross-tenant: tenant A cannot list, verify, or restore an archive belonging to B, a workspace of this installation; only the operator's `NEW_TENANT` lists and restores an archive of a workspace this installation does not hold (§8.2) |
| BK-11 | A `FULL` run with `trial_restore` reads its archive back and carries the report; one damaged before the trial fails with `backup.trial_restore_failed`; an older schedule keeps it off |

---

## 12. Open points

| # | Point | Needed by |
|---|---|---|
| B-1 | Whether `rclone` goes into the image (size, and its GPL-3.0 licence alongside Apache-2.0) | Before an `rclone` adapter |

**Object lock (B-3).** Required for the system backup target, recommended for a tenant's own. The
credential that writes backups must not be able to delete them or shorten their retention. For the
system backup the lock retention **equals** 35 days: longer and the plan's own cleanup fails against
the lock, shorter and [data-protection.md](./data-protection.md) §5 is not kept by the storage. A
tenant enabling it owes itself the same arithmetic.

Closed points: B-2 (system backups are the operator's) is §1 and §8.5; B-4 (the trial restore) §5;
B-5 (the `sync_epoch`) §8.3 step 6.
