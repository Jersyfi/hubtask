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

**The archive is streamed to the target as it is produced, not staged and then transferred** (E-04).
Both were open and both buy something real: staging makes a checksum over the whole archive trivial
and a resumption after process death cheap, because the finished part is on local disk; streaming
keeps memory and disk flat whatever the holding weighs. Streaming wins on the case that decides it —
this system runs in a container whose writable layer is small and whose data volume is the database,
and staging a hundred gigabytes there fails at the disk rather than at the target, which is the
worse of the two failures because it happens on the machine that is still working. What staging
bought is bought differently: the archive is a directory of members rather than one stream, so a
checksum per member replaces a checksum over the whole, and a resumption finds what is already at
the target with one `List` rather than on a scratch disk a restarted container no longer has.

Three consequences follow from that, and they are the format rather than the implementation:

* **`checksums.txt` is written last and is the commit point.** An archive without it is a run that
  died or one still going — not a damaged archive, and the difference is what §8.1 reports as the
  checksum status. The manifest names the members and their checksums and therefore cannot name its
  own, so `checksums.txt` closes over it. What that catches is corruption and not an attacker;
  against an attacker the defence is the authenticated cipher the members are written through.
* **The manifest is the one member that is never encrypted.** §8.1 requires the archives at a target
  to be listed with no state in the database at all, and an operator who has lost the database and
  holds the target credentials has not necessarily got the archive key yet. From that follows a rule
  with teeth: no user content in the manifest — counts are by entity, never by name — because
  whoever holds the storage can read it.
* **Media are counted, not listed.** A content-addressed file is named after the SHA-256 of its
  content, so a list of media checksums is a list of the file names; on a holding with a hundred
  thousand attachments it would be the largest thing in the archive. A medium lives in whichever
  archive of the chain first referenced it, and a restore resolves a digest by searching the chain
  from newest to oldest.

**Golden archives** are committed to the repository under `test/backup/golden/` — one per archive
format version, imported by BK-4, added at a major release ([versioning-release.md](./versioning-release.md) §1, §7).

---

## 4. Encryption

Backups sit, by definition, on somebody else's storage. Therefore:

* **Client-side encryption is the standard**, not an option: AES-256-GCM with a backup key per target (derived from a passphrase via Argon2id, or supplied directly). Neither of those two is available yet — a passphrase is not stored anywhere by design, and the surface that would take one is refused until a run exists to hand it to. Until then the key is **derived from the installation's master key with HKDF-SHA256, bound to the target** (E-05): two targets never share one, nobody has to remember a second secret, and the key that protects a backup is not itself a thing to back up. The master key's identifier goes into the manifest, which is what makes the rotation property below true for archives exactly as it is for sealed values.
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
fetched after the snapshot, using the referenced checksums — and their locations are resolved inside
the same snapshot, so the mapping from a content address to a storage key is the one the snapshot
saw and cannot change under the run.

Four things E-05 had to decide, and each is stated where the code is as well as here:

* **What anchors the rule.** A recurring task counts from its due date; a backup schedule has no due
  date, so **it counts from when the schedule was created**, read in its own zone. Counting from
  "now" at each expansion makes a rule without a `BYDAY` drift to whatever weekday the process
  happened to ask on — a weekly backup that moves because a pod restarted on a Tuesday. Counting
  from midnight of the creation day is a value nobody chose. For a rule that pins its own time —
  `FREQ=DAILY;BYHOUR=3` — the anchor does not matter at all.
* **`full_rrule` selects among `rrule`'s occurrences rather than producing occurrences of its own.**
  The example above names no hour in `full_every`: expanded on its own it would fire at whatever
  time of day the anchor carries, giving a full backup at a moment nobody scheduled *and* a second
  run beside the three o'clock one. Read as a filter it means what "every" means — the daily run on
  a Sunday is the full one. That also disposes of "what if both fall on the same instant" by making
  it impossible: only one of the two produces instants. The comparison is by calendar day in the
  schedule's zone, because a rule that names a day names a day.
* **The lock is the insert.** `INSERT … WHERE NOT EXISTS (… status = 'RUNNING' AND id <> …)` — a
  check followed by an insert has a gap between them wide enough for exactly the thing it prevents.
  The `id <>` is what makes a resumption possible: a worker that died left its own row RUNNING, and
  the attempt that takes the job over is the same run, so it must not be locked out by itself
  (BK-7). A second, different run answers "no", which is not an error: the work is happening.
* **Who fires what.** A tenant's schedules are fired by that tenant's own poller, seeded by the
  write that created one and rescheduling itself to the next moment the tenant owes — the shape
  every per-tenant job here has, because nothing may enumerate tenants
  ([multi-tenancy.md](./multi-tenancy.md) §2.1). An **instance-wide** schedule belongs to no tenant,
  so nothing seeds one, and firing it is the leader's duty. That is not a licence to enumerate
  tenants and cannot become one: the pass runs under a system scope, every tenant-scoped table
  compares against a tenant that scope does not have, and the only rows it can reach are the ones
  with none.

---

## 6. Retention of backups

Two levels, deliberately kept separate from the retention of business data
([data-retention.md](./data-retention.md)):

1. **The generation principle** (`keep_last`, `keep_daily`, `keep_weekly`, `keep_monthly`, `keep_yearly`) — as in established backup tools.
2. **`min_keep`** as a floor: a retention rule may never result in **no** backup being left. If a run fails, old archives are not deleted.

Expiry rules apply only to archives Hubtask created itself (recognised by the manifest); other files
at the target are never touched. Deletion is auditable. At targets with object lock/WORM, Hubtask
reports non-deletable archives as a notice instead of retrying endlessly.

Three things follow from that and are worth stating (E-05):

* **An archive another kept archive needs is kept.** An incremental restores only through its chain
  back to a full archive, so deleting a parent does not free one archive — it destroys every archive
  after it, silently, and the loss is discovered at the restore. This is not a retention rule; it is
  what makes the retention rules safe to run.
* **A run with no schedule behind it deletes nothing.** The plan lives on a schedule; a backup
  somebody asked for by hand has none, and inventing a default for it would mean the thing people do
  *before* something risky could delete the archives they were making it alongside.
* **What is at the target is read from the manifests, not from the database.** A row that says an
  archive exists is a row; the archive is what is at the target, and expiry deletes files. A file
  that is not a Hubtask archive is therefore never a candidate — the absence of a code path rather
  than a check. `checksums.txt` is removed first, so an interrupted deletion leaves something that
  reads as unfinished rather than as sound.

---

## 7. Backup and data protection

The conflict is well known: an erasure request takes effect immediately in the primary system, but
last week's archive still contains the data. How it is handled:

* Deletions are recorded in a **deletion journal**. On restore they are reapplied: objects deleted between the archive point and the restore do not come back. This is the most effective measure, because it works without access to old archives. The reader arrived with E-06, and it reads a window rather than the table: the journal outlives every archive, and an object deleted *before* the archive was taken is not in it, so what has to be kept out is what was deleted between the archive and now. A record that *points at* something the journal kept out is kept out with it — otherwise a restore obeying §7 would leave the workspace holding a row that references an object it deliberately did not create.
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
encryption key ID — and whether the run that wrote each one finished, which is the difference between
an archive that is damaged and one that is still being written.

What the route reads is the point of it. The only database access is the target's own row and its
sealed credential, which is what opening a connection needs and nothing more; the use case has no
run repository at all, so joining `backup_run` is not something the path could do by accident. At a
shared target the archive's own name is the filter, so one tenant is never told about another's —
and asking for another tenant's archives outright is refused rather than answered with an empty
list, which would be a wrong answer to a question nobody may ask.

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
at it, then decide. The API makes that the cheap path rather than a documented discipline: it is a
mode rather than a procedure, and the tenant it creates is minted by the use case rather than named
by the caller — which is what makes the job's elevation into it safe, because nothing of anybody
else's is under a tenant that did not exist a moment ago.

Four things E-06 had to decide about the table above:

* **Destructive is `REPLACE_TENANT` and `INSTANCE`, and deliberately not `MERGE` with `overwrite`.**
  An overwrite replaces the objects the archive names and leaves everything else; a replace removes
  what the archive does *not* name. That is the difference between losing an edit and losing a
  month, and only the second is worth a typed workspace name and a step-up in front of it.
* **`duplicate` applies to content, not to context.** An account is who somebody is, a label is the
  same label, a medium is the same bytes under a content address, and a webhook subscription copied
  is a subscription that fires twice to somebody who never asked. For those the rule falls back to
  `skip` and the report says so. What is copied — collections, buckets, labels, items and everything
  hanging off them — gets a **derived** identity rather than a drawn one, so that a resumed restore
  produces the same identifiers instead of a second copy of what it already wrote, and the copies
  point at each other rather than at the originals.
* **`SELECTIVE`'s closure comes out of the archive's reference graph.** A bucket names its
  collection, an item names its collection, a comment names its item — so "everything below the
  collection I named" falls out of the declarations. Only the containers need a pass of their own,
  because the order within an entity is by change time and a sub-collection can be written before
  its hub.
* **`INSTANCE` has nothing to restore yet, and is refused rather than approximated.** No archive
  this build writes has an instance-wide scope — B-2 leaves system backups to the operator until
  `0.6.0` — so the mode is accepted, the archive's manifest is read, and the scope check refuses it.
  The day an instance-wide archive exists the mode works without anything changing shape.

### 8.3 The procedure

1. Pre-check: checksums, schema/product version, decryptability, scope, estimated duration.
2. A dry run with a report (the number of new/overwritten/skipped objects, conflicts).
3. Confirmation by typing the tenant name for destructive modes; plus step-up authentication.
4. An automatic safety copy of the current state before destructive modes (if there is room at the target).
5. Execution as a job with progress; on cancellation, rollback within a transaction per batch size.
6. Follow-up: apply the deletion journal (§7), rebuild the search index, do **not** re-fire automation for the period (§8.4), and write the report to the audit.

Three of those steps are worth pinning down as E-06 implemented them:

* **Step 3's step-up is implemented since H-03** ([security.md](./security.md) §5). The seam E-06
  cut — `core/port/stepup` — is filled by a verifier that judges a fresh re-authentication on the
  current session, recorded there, valid for `HUBTASK_STEP_UP_WINDOW` and consumed by the one
  restore it is presented to. A destructive mode without the proof answers `403` with
  `auth.step_up_required` and the accepted methods; the "nothing here can prove it" refusal of
  E-06 died with the verifier that made a step-up satisfiable, and the fail-closed rule survives
  it — an unwired verifier still refuses rather than permits, kept by the tombstone test.
* **Step 4 is a refusal when there is nowhere to write the copy.** The step's own parenthesis is
  "if there is room at the target"; a destructive restore with no way back is the situation it
  exists to prevent, so "there was nowhere to write it" stops the restore rather than waiving the
  step. The copy's identifier is recorded on the run *before* the destructive mode runs, so the way
  back is findable even if the run then fails.
* **Step 5's batches carry their own progress marker.** Each batch commits its rows and how far the
  run has got in one transaction, so a worker that dies is replaced by one that continues rather
  than one that decides the same records again. That is what makes `duplicate` resumable at all:
  the question it turns on — "does the workspace already hold this" — changes its answer once the
  first attempt has written half the archive.
* **Step 6's index needs no pass of its own.** The search document is maintained by a trigger on
  the row, so rows written by a restore are indexed as they land.

### 8.4 What deliberately does *not* happen during a restore

* **No automation rules fire.** A restore would otherwise trigger hundreds of webhooks and emails effectively reporting old states. Restored changes produce events with `replay: true`, which the rule engine ignores. The flag arrived with E-06 rather than with the engine in `0.5.0`, because a flag introduced alongside the consumer that reads it has a window in which the consumer does not know about it — and that window is a restore. It is not delivered to a subscriber that has not asked for it: the decision is the dispatcher's, so a consumer added later by somebody who has never read this section is not handed a replay by accident.
* **No reminders are caught up** whose time lies in the past; they are marked as lapsed. `LAPSED` is a state of its own (migration 0037) rather than `CANCELLED`: nobody cancelled them, and an auditor reading a workspace after a restore should not find hundreds of cancellations that never happened.
* **No webhooks are re-delivered**; the archive's outbox is not imported.
* **No tokens or sessions are restored** — making credentials from an archive valid again is a security risk. Users must sign in again; PATs must be recreated. This is displayed before the restore. Neither table is in the archive at all, which is the stronger form of the same promise: a restore cannot write back what it was never given.

One entity is in the archive and is deliberately **not written back**: the audit trail. §4 of
[audit.md](./audit.md) makes the live trail a hash chain and E-09's `:verify` walks it; inserting
last month's entries into the middle of a live chain is not a restore but a rewrite, and "the trail
cannot be rewritten" is the property the whole audit surface rests on. `include_audit` still puts it
in the archive, which is what it was for — the evidence is readable where it was written down.

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
| Signal | Meaning |
|---|---|
| `hubtask_backup_last_success_timestamp_seconds` (metric) | When each target last had a backup that worked. Emitted by the leader since E-05, labelled by target, and a timestamp rather than an age so that the alert computes the age at evaluation time rather than at scrape time. A target that has never had one is **absent** rather than zero — a gauge of zero reads as 1970 |
| `hubtask_restore_drill_last_success_timestamp_seconds` (metric) | When a trial restore last worked. **Still emitted by nothing**, and A-20 stays dormant because of it: E-06 built the restore and E-12 built the drill somebody runs, and neither writes the gauge. What would write it is a successful non-destructive restore recording its own timestamp; until that exists, an installation proves its drills through `hubctl` and the run rows, not through the alert |
| A-12 | No successful backup in 24 hours, **per target** — a `max()` across targets would let one healthy target hide a broken one, which is exactly the 3-2-1 arrangement §2 recommends |
| A-20 (new) | The restore drill is older than 90 days |

**A-20 rather than A-19**, and the renumbering is this way round on purpose. `data-protection.md` §4
and [ADR-0018](../adr/ADR-0018-privacy-by-design.md) had already given A-19 to the data subject
request deadline; an ADR records a decision that was taken, and editing one so that a later table
can keep its number is the wrong direction. The restore drill is the newcomer, so the restore drill
moves (E-05).

The drill itself is deliberate: a backup that has never been restored is a hypothesis. The regular
`NEW_TENANT` trial restore can be automated and evaluated as a test run. Its alert is a ticket
rather than a page, and it does not fire on the metric's absence the way A-12 does — nothing records
a drill yet, and an absence rule would page every installation for a feature that does not exist.

**What runs one** (E-12). The client is the drill:

```bash
hubctl backup run --target "$TARGET" --follow --wait 30m
hubctl backup verify "$RUN" --follow
hubctl restore inspect --target "$TARGET" --archive "$ARCHIVE"
hubctl restore run --target "$TARGET" --archive "$ARCHIVE" --mode NEW_TENANT --apply
```

`scripts/hubctl-e2e.sh` runs exactly that against the reference Compose stack on every pull
request, and it compares the number of entries in the restored workspace with the number in the
source — from the database on both sides, so that nothing in the check can agree with itself. That
is the first proof outside a test that the whole chain survives a round trip: schedule, job,
encryption, target, manifest, listing, restore.

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
| B-5 | What a restore owes connected devices. It writes rows without change log entries, so a device that was offline through one keeps a cursor that is still valid and will never be told what changed (E-06, [offline-sync.md](./offline-sync.md) §8). The candidates are a change log entry per restored row, or a per-tenant "resynchronise from scratch" marker that a pull turns into a full sync — the second is cheaper and is probably right for an act this rare, and neither should be guessed at inside a backup task | `0.5.0` |
