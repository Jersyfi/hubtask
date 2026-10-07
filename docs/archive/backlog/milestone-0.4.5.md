# Milestone 0.4.5 — Backup, retention, audit

The goal: the system becomes accountable for the data it holds. An archive leaves the process
encrypted and lands at a target the operator chose; it can be listed, verified and restored when
the database that produced it is gone; a retention rule announces what it will delete before it
deletes it, and can be stopped; a legal hold outranks every rule; an auditor can read the trail,
export it, and be told whether it has been tampered with; and a person who asks what is held about
them gets an answer within a deadline the system watches. Phase 1 closed with 0.4.0 — the core is
functionally complete per the product idea. This milestone is what makes it operable.

Every task is one pull request. The order is binding where dependencies exist.

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue.

What deliberately is **not** in this milestone: the `ftps`, `ftp`, `smb`, `azure`, `gcs` and
`rclone` target adapters — the roadmap gates them on BK-1 and `rclone` additionally on open point
B-1 (`0.5.0`, its GPL-3.0 licence against the BSL distribution); orchestrated system backups and
PITR (B-2, `0.6.0` — the operator's job until then); tenant export `GET /tenants/{id}:export`,
quotas and provisioning (`0.6.0`); the master key in a KMS or Vault (S-2, `0.6.0` — see decision 3);
the legal settling of the audit period (A-1) and the defaults P-5 and P-6 (`0.6.0`); external chain
anchoring into a WORM target, for which `audit_anchor` has stood empty since `0001_init` (A-2,
`0.9.0`); a SIEM connection (A-3, after `1.0.0`); the importers that share the archive's ingestion
path (`0.9.0`); and every screen that renders any of it — the client builds this surface in **F4**,
one window behind, exactly as `roadmap.md` says it should.

Six decisions taken while writing this backlog, so that nobody re-derives them later:

* **The contract is already written; this milestone implements it.** `0001_init` carries
  `backup_target`, `backup_schedule`, `backup_run`, `restore_run`, `deletion_journal`,
  `retention_policy`, `retention_run`, `legal_hold`, `audit_log` with its partitions, triggers and
  grants, `data_subject_request`, `consent_record` and `privacy_incident`. `openapi.yaml` carries
  seven backup paths, four retention paths and two audit paths, all generated into
  `presentation/openapi`. Ten of them are registered stubs answering `route.operation_not_available`
  — and `presentation/rest/Pending.go` says what that file is for: "the visible remainder of the
  milestone: what is still missing is a list, not a guess." What is absent is every port, every
  adapter, every job kind, every sqlc query over those tables, and every test: `core/port/
  backupstorage/`, `infrastructure/backupstorage/`, `test/backup/` and `test/audit/` contain a
  `.gitkeep` and nothing else. So the specification-first step of most tasks here is small, and the
  ones that need it — `POST /audit:export`, the legal hold, the data subject request — are named in
  their task.
* **`/jobs/{id}` opens the milestone, and 0.4.0 said when it would.** That backlog deferred the
  resource with a condition rather than a date: "the `/jobs` resource waits for the first operation
  that genuinely cannot be bounded." A backup is that operation. `startBackup`, `verifyBackup` and
  `startRestore` already answer `202 Accepted` with a `JobRef` that points at nothing, so E-01 is
  not new scope — it is the debt those three responses have been carrying since A-06.
* **`golang.org/x/crypto` is the milestone's one new direct dependency, and three documents chose
  it first.** `backup-restore.md` §4 names AES-256-GCM and Argon2id, `security.md` names envelope
  encryption for stored credentials, and the `sftp` adapter needs the same module's SSH client. It
  is already an indirect dependency; E-02 promotes it and pins it, and no other task introduces
  any. **Open point S-2 does not block this.** S-2 asks where the master key lives *in provider
  operation* and is scheduled for `0.6.0`; E-02 takes it from the environment, persists the key ID,
  and builds rotation now because §4 requires old archives to stay readable with the old key.
  Someone reading S-2's `0.6.0` would otherwise conclude that backup encryption is blocked, and it
  is not.
* **Backup retention and data retention are two mechanisms sharing one word, and they stay
  separate.** `backup-restore.md` §6 is the generation principle — `keep_last`, `keep_daily`,
  `keep_weekly`, `keep_monthly`, `keep_yearly`, with `min_keep` as a floor that may never leave
  zero archives behind. `data-retention.md` is a day-count engine over business data with actions,
  chains and safeguards. E-05 owns the first, E-07 the second; neither reads the other's table, and
  no task may generalise them into one engine.
* **The audit evidence catalogue is called `AT-n` in one document and `AU-n` in six, and E-09 ends
  that.** `audit.md` §8 lists AT-1…AT-7. `roadmap.md`, `CLAUDE.md`, `ci-cd.md`,
  `engineering-guidelines.md`, `project-structure.md` and `versioning-release.md` all say
  AU-1…AU-7 — and the two do not map: `versioning-release.md` makes AU-1 "every action marked
  auditable produces exactly one entry", which is the SG-13 declaration gate, while `audit.md`
  makes AT-1 the grants test. `roadmap.md` release criterion 11 requires "AU-1…AU-7 green", and a
  release criterion nobody can evaluate is not a criterion. E-09 picks one prefix, rewrites the
  other occurrences in the same change, and records in `audit.md` which old identifier became
  which.
* **Gates PG-1…PG-8 are asserted by four documents and exist in no form at all** — not a Makefile
  target, not a script, not a test; the strings appear only in prose. C-09's acceptance already
  claimed "PG-7 stays green" against nothing. E-11 builds them, and until it lands **no task in
  this milestone may name a PG gate in its acceptance** — it would be the same false green a second
  time.

---

## E-01 — `/jobs/{id}`: the resource three responses already promise **[L]**

*Depends on: nothing. The first task, and the one the backup half cannot start without.*

`GetJob` behind `GET /jobs/{jobId}` and `CancelJob` behind `POST /jobs/{jobId}:cancel`, over the
`job` table A-08 built. The `JobRef` schema has existed since A-06 with `job_id`, `status`,
`progress` and `result_url`; what has never existed is the resource it refers to, and
`api-guidelines.md` §5 has been naming `202 Accepted` plus `/jobs/{id}` as the long-running shape
since before there was a long-running operation. Both routes are the specification-first step.

The reason it lands now and did not land in 0.4.0 is the condition that backlog wrote rather than
the date it did not: a template instantiation and a view export can be bounded, and were; a backup
of a tenant's holdings cannot be, and neither can a restore or a checksum pass over an archive at a
remote target. Three operations in the specification already answer `202` with a `JobRef`, which
means the contract has been promising this resource to clients that had nothing to poll.

What the task must decide out loud is how much of the `job` row becomes public. The table carries
`kind`, `payload`, `attempts`, `locked_until`, `dedupe_key` and an error column, and most of that
is the queue's business rather than a caller's: a payload can hold identifiers a caller may not
resolve, and `attempts` invites a client to reason about the retry policy. The narrow reading —
status, progress, a result reference and a message code on failure — is the one the schema already
describes, and widening it later is additive while narrowing it is not. Authorisation is the
application layer as always (rule 2): a job is visible to the tenant it belongs to and to the actor
who caused it, an instance-scoped job only to instance administration, and cancellation is a
distinct permission from reading, because "show me" and "stop it" are different questions.
Cancellation is cooperative — it sets a state the handler observes at its next batch boundary, not
a signal that kills a pass mid-write — and a job already finished answers `409` rather than
pretending.

**Acceptance:** `GET /jobs/{jobId}` answers status and progress for a job the actor may see, `404`
for one belonging to another tenant with a body indistinguishable from a job that never existed,
and the cross-tenant negative test proves it; `POST /jobs/{jobId}:cancel` moves a `PENDING` or
`RUNNING` job to `CANCELLED` and is refused with `409` on a terminal one; no payload field, no
`dedupe_key` and no attempt count leaves the boundary, proved by a contract test against the
response schema; the two use cases are registered in the registry and reachable through REST, MCP
and automation with the parity gate green; both carry a metric, a span and an audit declaration;
`progress` is honest — a handler that cannot report it returns `null` rather than a number it
invented.

**Read:** `api-guidelines.md` §2, §5, §6; ADR-0008; A-08 in `milestone-0.1.0.md`; the `/jobs`
decision in `milestone-0.4.0.md`

---

## E-02 — Envelope encryption: the port, the key, and its rotation **[L]**

*Depends on: nothing. Runs alongside E-01.*

An `Encryptor` port in `core/port` and an adapter in `infrastructure/crypto`: AES-256-GCM with a
data key per target, the data key sealed with a master key the environment supplies, and the key
identifier persisted so a rotation does not migrate data. `0001_init` has been holding the columns
open since phase 0 — `credential_enc`, `credential_key_id`, `encryption_mode` and
`encryption_key_id` on `backup_target`, with two table checks that already fail closed: an
unencrypted target and an FTP target each require `insecure_ack_at`. The milestone's one new direct
dependency lands here (decision 3) and nowhere else: `golang.org/x/crypto` for Argon2id, which
`backup-restore.md` §4 names for deriving a backup key from a passphrase.

Three rules from §4 are the substance, and each of them is a property of this task rather than of
the adapter that later uses it. **The key is not stored in the archive** — without it the backup is
useless, which is stated as an unmissable notice at setup and logged on confirmation, and it is why
`BackupTargetCreate.encryption_passphrase` is documented in the specification as not stored. **The
key can be rotated and old archives stay readable**, which is what the key identifier in the
manifest is for and what makes rotation a design constraint now rather than a feature later.
**Unencrypted is possible and never quiet**: `encryption: none` requires explicit acknowledgement
and produces a permanent warning in `/meta/health`.

The rule 10 boundary is absolute here in a way it is not elsewhere: a key, a passphrase, a data key
and a plaintext credential appear in no log, no metric, no trace, no audit `changes` and no error
message. The `secret.Secret` wrapper T-18 already provides is the type these values travel in, and
a value that reaches the adapter outside it is a bug the tests look for. What the task decides is
the shape of the master key configuration — one key with an identifier, or a small keyring with a
current key and readable predecessors — and the answer is the second, because rotation without a
readable predecessor is not rotation. The environment is the only source in this milestone; S-2
decides the rest at `0.6.0`, and the port is the seam that makes that a new adapter rather than a
rewrite.

**Acceptance:** a value sealed with key `a` and rotated to key `b` still opens with `a` recorded in
its key identifier, and a value sealed with `b` does not open with `a`; a wrong key fails with a
typed error and never a partial plaintext; ciphertext is distinct for identical plaintext, proved
by a nonce test; no key, passphrase or plaintext credential appears in any log, metric, trace or
audit entry, proved by the SG redaction tests extended to the new type; Argon2id parameters are
named constants with the reasoning for their cost written where they are declared;
`golang.org/x/crypto` is promoted to a direct dependency with a pinned version and the licence gate
stays green; the domain and application layers see only the port, and `gate-architecture` proves
`crypto/cipher` appears in no layer inwards of infrastructure.

**Read:** `backup-restore.md` §4; `security.md` §2, §3 and open point S-2; ADR-0015; ADR-0019
decision 4; T-18 in `security.md` §4

---

## E-03 — The backup target: the port, four adapters, and BK-1 **[L]**

*Depends on: E-02 — a target's credentials are stored encrypted or not at all.*

`core/port/backupstorage/Port.go`, the file `backup-restore.md` §35 names and the directory that
has held a `.gitkeep` since phase 0, with adapters for `local`, `s3`, `sftp` and `webdav`; plus
`CreateBackupTarget`, `ListBackupTargets` and `TestBackupTarget`, whose routes are already
specified and already stubbed. The four are the roadmap's opening set, and the remaining seven
adapters ship when they pass this task's conformance suite — ADR-0019 makes that a gate rather than
an aspiration: "new adapters must pass it or they do not ship."

The port is **not** `core/port/storage`. C-05's object store is media-shaped, and the mismatch is
not stylistic: `Upload.Size` is a declared exact length, documented as "the size is declared rather
than discovered", and an archive's length is not known before it is written; `TransferIssuer` is
typed on `media.Object`, a domain type a backup has no business carrying. What does transfer is the
pattern — the hand-signed SigV4 from `infrastructure/storage/SigV4.go` rather than an AWS SDK, the
resilience wrapper, the health registration under a feature name, and the outbound exception
precedent for a self-hosted endpoint on a private network.

The security shape is the sharpest part of this task, because a backup target is by definition an
egress channel and `backup-restore.md` §2 makes it a **narrow, named exception** to the SSRF rule
rather than a hole. Targets may be created only by instance administrators; tenant-owned targets
are a switch that is off by default (`HUBTASK_BACKUP_TENANT_TARGETS`, read by nothing today) and
carry an egress allowlist on top when on; `:test` is a write-read-delete probe that runs through
`GuardedClient` with metadata endpoints and private ranges blocked unless released (rule 6, and
BK-9 proves it); and every target change is auditable under `backup.target_changed`. The
unencrypted and FTP acknowledgements are enforced in the domain as well as by the check constraint,
because a constraint produces a database error rather than a field error with a message code.

Two naming defects get fixed here rather than inherited. The specification's `BackupTarget.warnings`
gives examples `backup.target_unencrypted` and `backup.target_plaintext_protocol`, while §10 names
`config.backup_unencrypted` and `config.backup_single_target` for the health surface; the task
decides which vocabulary is which — a warning on a resource and a warning on the installation are
different things and may legitimately differ, but not by accident. And `EnvConfig.go` reads
`HUBTASK_BACKUP_TARGETS`, an environment variable documented nowhere; the task either documents it
or removes it.

**Acceptance:** BK-1 is one conformance suite run against all four adapters under Testcontainers —
write, list, read, verify, delete, and the same behaviour on a re-run — and it is the gate a fifth
adapter passes before it exists; BK-9 proves a target pointed at `169.254.169.254` or a private
range is blocked by `GuardedClient` and released only by explicit configuration; credentials are
written encrypted through E-02's port and are never returned by any read, proved by a contract
test; an unencrypted or FTP target without `insecure_acknowledged` is refused with a field error
and a message code, and the acknowledgement is audited with who gave it; `config.backup_not_
configured` clears when a target exists and `config.backup_unencrypted` appears when one is
unencrypted, both with locale entries; `test/backup/` stops being a `.gitkeep` and `gate-data`
starts earning its help line; the three use cases are registered and the parity gate is green.

**Read:** `backup-restore.md` §2, §10; ADR-0019 decisions 2 and 5; `security.md` §4 (T-07),
ADR-0015; C-05 in `milestone-0.3.0.md`

---

## E-04 — The archive: manifest, JSON Lines, content-addressed media **[L]**

*Depends on: E-02, E-03. The format, before anything writes one on a schedule.*

The Hubtask archive of `backup-restore.md` §3 as a reader and a writer: `manifest.json` with the
format, schema and product versions, scope, period, counts, encryption and parent; `data/*.jsonl`
per aggregate; `media/<sha256-prefix>/<sha256>` content-addressed and deduplicated; `checksums.txt`
per file and over the manifest. This is a format decision expressed as code, which is why it is its
own task and comes before the job that produces one on a timer.

Four properties carry the weight. **JSON Lines rather than an SQL dump**, so that an archive from
1.2 opens in 1.7 through the same upward migrations domain objects take — a `pg_dump` would bind
the archive to a PostgreSQL version and a schema version at once. **Content-addressed media**, so
that an incremental run does not re-transfer a file that did not change. **Incremental against a
parent** on `updated_at`/`seq`, carrying deletions as tombstones — an incremental chain that
omitted them would resurrect deleted objects on restore, which is the defect BK-3 and BK-6 exist to
catch. **Encrypted before it leaves the process**, through E-02, not at the target.

Consistency is §5's `REPEATABLE READ` snapshot: the export represents one point in time rather than
a mixture, `backup_run.snapshot_at` records it, and media are fetched after the snapshot by the
checksums the snapshot referenced. The task's own decision is where the archive is assembled —
streamed to the target as it is produced, or staged and then transferred. Streaming keeps memory and
disk flat and is what an unbounded holding requires; staging makes a checksum over the whole archive
trivial and a resumption after process death cheap. Whichever wins, the reasoning is written down,
because BK-7 tests the answer.

Golden archives are a deliverable of this task and not of a later one: one per major version, in
the repository, imported by BK-4. The first one is written here, and the rule that every major
version adds its own goes into `versioning-release.md` where the release checklist can see it.

**Acceptance:** BK-2 — an encrypted archive is unreadable without its key, and after a rotation the
old archive still opens with the key its manifest names; BK-3 — an incremental chain of ten runs
including deletions reproduces the source state exactly; BK-4 — the golden archive in the
repository imports, and the rule for adding one per major version is in `versioning-release.md`;
the manifest is versioned and a reader refuses a format version it does not know with a typed error
rather than a partial import; checksums cover every file and the manifest, and a corrupted byte is
found; the snapshot is `REPEATABLE READ` and a write concurrent with an export appears wholly or
not at all in the result; no user content reaches a log, a metric or a trace while an archive is
written (rule 10 applies to the exporter exactly as to everything else).

**Read:** `backup-restore.md` §3, §5, §11; ADR-0019 decision 3; `offline-sync.md` §7 (tombstones);
`versioning-release.md` §3

---

## E-05 — Backups run: the job, the schedule, generational retention, `:verify` **[G]**

*Depends on: E-01, E-04. The first thing in this milestone that happens without anybody asking.*

`StartBackup`, `VerifyBackup` and `CreateBackupSchedule` — all three specified, all three stubbed —
plus the `backup.run` job kind and the generational expiry of §6. The schedule reuses the RRULE
engine D-04 built and does not build a second one: `backup_schedule.rrule` has carried the comment
"RFC 5545, the same engine as recurrence" since `0001_init`, `backup-restore.md` §5 says the same in
prose, and `core/port/recurrence` is free of the work domain — a rule text, an IANA zone, a window
and a limit. Two things it does not answer, and the task must: what anchors `DTSTART` when a
schedule has no due date, and what happens when `rrule` and `full_rrule` fall on the same instant.

The scheduling shape needs a decision stated in the pull request, because this milestone is where
the "nothing enumerates tenants" rule meets its first legitimate exception. Every per-tenant job so
far is self-seeding: the write that creates the work seeds the job for its own tenant, and
`EnqueueJob`'s `LEAST(run_at)` clause pulls a wake-up forward rather than adding a row. A
tenant-scoped backup schedule follows that shape unchanged. An **instance-scoped** schedule —
`scope_kind = 'INSTANCE'`, `tenant_id IS NULL`, which the table's own check constraint ties
together — is not a tenant's work at all, and `backup_schedule_due_idx ON (next_run_at) WHERE
enabled` was built for a leader to read. That is a legitimate leader duty and the first one the
scheduler has beyond sampling queue depth; it is not a licence to enumerate tenants, and the task
says so where the code is.

The run itself is `Detached` — the one job shape the runner does not wrap in a transaction, "for
the one kind of job that cannot live inside it: a pass that has to reach a bucket between two
writes" — with progress against E-01's resource, cancellation, resumption after process death, and
a lock against two runs on one target. §5's throughput rule is not optional: a running backup must
not slow interactive work, so reads are throttled on a bulkhead pool separate from the API path.

Expiry is the generation principle and nothing else (decision 4): `keep_last`, `keep_daily`,
`keep_weekly`, `keep_monthly`, `keep_yearly`, with `min_keep` as a floor a rule may never breach,
no deletion at all after a failed run, only archives Hubtask itself created — recognised by their
manifest, never by a filename — and never another file at the target. Where the target holds an
object lock, a non-deletable archive is reported as a notice rather than retried forever.

The observability half closes two things that have been open since phase 0.
`hubtask_backup_last_success_timestamp_seconds` is the metric alert A-12 has been watching and
nothing has ever emitted; RB-A12's own runbook calls its absence "the honest state before `0.4.5`".
And A-19 — the restore drill older than 90 days — gets its rule, its runbook and its metric.
`data-protection.md` §4 and ADR-0018 use `A-19` for the data subject request deadline alert as well;
one of the two is renumbered here, in the same change, rather than left as two alerts with one
identifier.

**Acceptance:** a schedule at `FREQ=DAILY;BYHOUR=3` in `Europe/Berlin` fires at 03:00 local through
both DST transitions, proved against D-04's golden expectations; BK-7 — process death during a
backup resumes without a duplicate archive and without a duplicated media object; BK-8 — the
generation plan deletes exactly what it should, `min_keep` is never undercut, a failed run deletes
nothing, and a foreign file at the target is untouched; `:verify` checks checksums and
decryptability at the target without restoring and records `verified_at`/`verify_ok`; the metric
A-12 watches is emitted per target and the alert is exercised against it; A-19 has a rule, a
runbook and no shared identifier; `backup.downloaded` and `backup.target_changed` are registered
audit actions; the three use cases are registered with metric, span and audit declaration, and the
parity gate is green.

**Read:** `backup-restore.md` §5, §6, §10; ADR-0008; `observability-reliability.md` §3, §8;
D-04 and D-05 in `milestone-0.4.0.md`; `multi-tenancy.md` §2.1

---

## E-06 — Restore: listing at the target, six modes, the deletion journal **[L]**

*Depends on: E-04, E-05. Security-critical: the milestone's only destructive path.*

`ListBackupsAtTarget` and `StartRestore`, and with them the first reader the `deletion_journal` has
ever had — a table written since B-10 and read, until now, only by tests, with a comment saying so:
"nothing reads this table in production; it exists so that a restore from backup cannot bring back
what was deleted."

The listing is the part that decides whether any of this is worth having. §8.1 requires it to read
the manifests **at the target** and to need no state in the database, "so that a restore works even
when the database is lost and only the target credentials exist" — which means the listing path may
not join a `backup_run` row, and a test proves it by listing against a target with an empty
database. `backup_run` stays what its own comment calls it: "a log and an accelerator, not a
prerequisite for a restore."

Six modes, and they are not variations on one code path: `INSPECT` changes nothing and reports the
difference; `SELECTIVE` pulls named containers or items back into the living tenant; `MERGE`
imports with `skip`, `overwrite` or `duplicate` per collision; `REPLACE_TENANT` resets a tenant to
the archive; `NEW_TENANT` imports alongside; `INSTANCE` restores a system backup under maintenance.
§8.2 names `NEW_TENANT` as the way to check before a destructive mode, and the API should make that
the easy path rather than a documented discipline. The procedure of §8.3 is a checklist the code
follows in order: pre-check, dry run with a report, the tenant name typed into `confirmation` plus
step-up authentication for destructive modes, an automatic safety copy before them, execution with
progress and per-batch rollback, and then the follow-up — reapply the deletion journal, rebuild the
search index, and write the report to the audit.

§8.4 is four prohibitions and each is a test rather than a promise. **No automation fires**:
restored changes carry `replay: true` and the rule engine ignores them, which means the event
envelope grows a field in this task and the engine that will read it in `0.5.0` finds it already
there. **No lapsed reminder is caught up** — it is marked lapsed. **No webhook is re-delivered** and
the archive's outbox is not imported. **No token and no session is restored**, because making
credentials from an archive valid again is a security defect wearing a recovery feature; people
sign in again, PATs are recreated, and the API says so before the restore rather than after.

Step-up authentication is the one thing here that has no implementation to build on — sessions and
MFA are `0.6.0`. The task does not build them: it defines the seam, refuses a destructive mode
without a satisfied step-up, and where the installation cannot yet satisfy one, a destructive
restore is refused rather than silently permitted. A confirmation that is structurally impossible
to give is a stronger position than one that is skipped.

**Acceptance:** BK-5 — a restore fires no automation, sends no webhook and no email, and restores
no token or session, each proved by a spy rather than by inspection; BK-6 — an object deleted after
the archive was taken does not return, through the deletion journal, for a row and for a media
object; BK-10 — tenant A cannot list, verify or restore an archive belonging to B, at the listing,
at the dry run and at the execution; the listing works against a target with an empty database; a
dry run produces a report of new, overwritten, skipped and conflicting objects and changes nothing,
proved by a checksum of the tenant before and after; a destructive mode without the typed tenant
name, or without step-up, is refused with a stable code; the safety copy is taken before a
destructive mode and its identifier is on the `restore_run`; BK-7's restore half — process death
mid-restore resumes without duplicates; the report reaches the audit and the use cases are
registered with the parity gate green.

**Read:** `backup-restore.md` §7, §8; ADR-0019 decisions 6 and 7; ADR-0007 (the event envelope);
`security.md` §2, §5; `automation.md` §1.3

---

## E-07 — Retention becomes the engine ADR-0020 describes **[L]**

*Depends on: E-03 — `EXPORT_THEN_DELETE` writes its archive to a backup target.*

The retention engine today enforces two data kinds, `TRASH` and `NOTIFICATION`, from a table whose
primary key is `(tenant_id, data_kind)`. That was deliberate and it is written down where it was
decided: "a constant for a kind nothing removes would be a promise nothing keeps." This task turns
it into the rule model of `data-retention.md` §2 — and the table cannot hold that model, so the
migration comes first: an identifier of its own, `scope_kind`/`scope_id` for the tenant/hub/
collection precedence, an optional CEL condition, an `action`, `then_after_days`/`then_action` for
chains, `grace_days`, `notify`, `enabled` and `export_target_id`. Forward-only and expand before
contract (rule 12): the new shape arrives alongside the old key, the seeded defaults are carried
over, and nothing rewrites `0001_init`.

Two-phase execution is the heart of it. Phase one marks and warns — `retention_pending_until`, the
advance notice, and the object carrying `retention: { action, effective_at, policy_id, can_retain }`
so that every client can see what is coming; phase two executes when the grace period has elapsed.
In between, anybody with permission takes an object out by editing it, moving it, or calling
`:retain`. `retention_run.phase` has allowed `MARK` and `PREVIEW` since `0001_init` and has only
ever been written `EXECUTE`; the reasoning for that is in `db/queries/Lifecycle.sql` and is worth
keeping — the trash is its own grace period, so `MARK` belongs to the kinds that have no trash of
their own, and the task should not put a second grace period on top of the first.

The safeguards are a precedence order, not a set, and the first match wins: legal hold; a data
subject request restricting processing; the lower bound per kind; the upper bound, where exceeding
it requires a `justification` and produces an audit entry; the minimum tombstone period, so that a
device offline for sixty days does not resurrect what was deleted; and the referential safeguards
that work a chain from the bottom up. Two of those have no implementation to lean on yet — E-08
builds the placeable hold and E-10 the restriction — so this task builds the precedence with the
seams and those tasks fill them, rather than each task inventing its own ordering.

`:preview` and the five-per-cent switch are what make a wrong rule survivable: a preview reports the
count, the share of the holdings and sample objects before a rule is ever active, and a newly
activated rule whose first run would touch more than five per cent of the holdings starts in
`NOTIFY_ONLY` with a clear notice. `data-retention.md` §6 names
`GET /retention-policies:effective?container_id=…` while the specification implements the same
question as query parameters on `GET /retention-policies`; the two say the same thing and the task
makes one of them the wording, in the document rather than in a comment.

**Acceptance:** RE-4 — an object marked in phase one and taken out with `:retain` is not deleted in
phase two; RE-7 — the first activation of a broadly matching rule warns instead of deleting, and
the share it reports matches what a preview said; RE-9 — a chained rule passes correctly through
completed → archive → deletion, with the anchor of each stage taken from the right column; RE-1,
RE-2, RE-3, RE-5, RE-6 and RE-8 stay green and RE-3 gains its upper-bound half — exceeding
`max_days` demands a `justification` and writes an audit entry; the data-kind catalogue is
configurable without an engine change, and a kind nothing sweeps is still refused rather than
accepted silently, keeping `lifecycle.history_not_wired`'s reasoning intact; `retention` appears on
the object through the API for as long as a rule applies to it; the block-reason metric covers
every kind that can be blocked rather than `TRASH` alone; the new fields have merge rules in
`offline-sync.md` §4.2 and the new personal-data rows are in the data catalogue.

**Read:** `data-retention.md` §2, §3, §4, §5, §6, §7; ADR-0020; `offline-sync.md` §7;
`automation.md` (CEL); B-10 in `milestone-0.2.0.md`

---

## E-08 — Legal hold, and the safeguard that can finally be placed **[G]**

*Depends on: E-07 — it is the first entry in that precedence order.*

`PlaceLegalHold`, `ReleaseLegalHold` and `ListLegalHolds`. The read half has existed since B-10 and
works: `ActiveLegalHolds`, `Holds.Blocking`, the refusal with `lifecycle.legal_hold`, the count in
`retention_run.blocked_reasons`, and a test that proves a purge is stopped. What has never existed
is a way to place one — every hold in every test is an `INSERT` written by the test — and
`LegalHold.go` says so: "Lifting one is auditable; that happens where holds are placed, which is
not this task." It is this one.

Nothing is in the specification, so the routes are the specification-first step, CRUD-shaped per
`api-guidelines.md` §2. The domain model needs three fields it does not carry — `PlacedBy`,
`ReleasedBy`, `ReleasedAt` — because the columns exist and the audit obligation is about who, not
only what. Authorisation is narrow and deliberate: a hold overrides a tenant's own configured
periods and a person emptying their own trash, so placing one is not an ordinary member's power,
and releasing one is audited with the reason.

The `ACCOUNT` scope is the decision this task has to take rather than inherit. The check constraint
accepts it, and `Holds.Blocking` deliberately ignores it with a comment pointing at where a
person's own data is erased — which is E-10. Either this task answers an account hold where E-10
erases, or it refuses the scope until E-10 lands. Accepting a value the engine silently ignores is
the one option that is not available: a hold that is stored and not honoured is worse than a hold
that was refused, because somebody believes it is in force.

**Acceptance:** a hold placed through the API stops a retention run and a manual purge, and appears
as `legal_hold` in the run's blocked reasons and on the object — QS-23 demonstrated end to end
rather than by construction; releasing one writes `released_by` and `released_at` and produces an
audit entry with the reason; a hold on a container reaches everything below it and a hold on an item
does not reach its siblings, extending the existing `LegalHold_test.go` cases; the `ACCOUNT` scope
is either honoured or refused, with the choice recorded in the pull request and in
`data-retention.md`; the three use cases are in the catalogue in `domain-model.md` §5, registered,
and the parity gate is green; the cross-tenant negative test proves a hold in one tenant is
invisible and inert in another.

**Read:** `data-retention.md` §4; ADR-0020 decision 4; arc42 §9 QS-23; `audit.md` §4;
B-10 in `milestone-0.2.0.md`

---

## E-09 — The audit becomes readable: query, export, `:verify` **[L]**

*Depends on: E-01 — an export over a 400-day period is the second operation that cannot be bounded.*

`QueryAuditEntries` behind `GET /audit`, `ExportAuditTrail` behind `POST /audit:export`, and
`VerifyAuditChain` behind `POST /audit:verify`. Two of the three are specified and stubbed; the
export is in no specification at all, although `audit.md` §5 has made it binding since the document
was written, so it is the specification-first step. The write path has worked since phase 0 —
`AuditSink.Append`, the advisory lock per tenant, the chain, ninety-one action codes across
ninety-seven registered use cases. There is not one line of read SQL over `audit_log`.

`:verify` is the task's centre and most of it is already prepared. `Canonical` was exported for this
and says why: "a verifier that used a second implementation would prove the two implementations
agree rather than that the chain is intact." The response shape is specified — `valid`, `checked`,
`first_broken_seq`, `gaps`, `sealed_until` — and `0001_init` explains why gap detection lives in the
application: a global `UNIQUE (tenant_id, seq)` cannot be enforced across partitions. AT-2 exists
today "in its first form", three entries; this is where the thousand-event run and the tampered row
arrive.

The access model is §5 and it is not the ordinary role matrix. A tenant `OWNER` or `ADMIN` reads
their own tenant's trail; a `MEMBER` reads their own events, which is transparency towards the
employee rather than a lesser admin view; an instance administrator has no blanket insight into a
tenant trail without a documented occasion, and that occasion is itself audited. The `AUDITOR` role
exists because the alternative in practice is handing an auditor administrator rights, and it reads
the trail and the configuration and no content. The role does not exist in code or schema today and
this task adds it, along with the `audit:read` scope the specification already requires. Every
export produces an audit entry of its own — §5's last line, and the first `audit.*` self-audit
action the system has.

Three things get fixed here because a reader of these documents currently cannot get a straight
answer. The `AT-`/`AU-` split (decision 5) is resolved and every occurrence rewritten in the same
change. `docs/audit/event-matrix.md` is named by §4 as the full matrix and does not exist; it is
generated from the registry — the registry is the only honest source, and generating it means it
cannot drift. And the pseudonymisation rule that `data-retention.md` and ADR-0020 both cite as
"audit.md §6" is not in §6, or anywhere: the audit is exempt from deletion and pseudonymises
instead, and this task writes that down where two documents already point.

The partition question rides along because it is a correctness matter and not housekeeping. There
is one real monthly partition, from August 2026, plus a default; the retention job drops whole
partitions rather than rows; and `0001_init` records a measured finding — a policy on the parent is
not inherited when a partition is addressed directly, so every partition created later must carry
its own RLS policy and its own revoked grants. A partition-creation duty that forgets either is a
cross-tenant leak with a date on it.

**Acceptance:** AT-1 — `UPDATE` and `DELETE` under the app role fail against both the grant and the
trigger, on the parent and on a partition; AT-2 — the chain and `seq` are gapless over a thousand
mixed events, and a row tampered with directly in the database is found by `:verify` with the right
`first_broken_seq`; AT-3, AT-4 stay green and AT-5, AT-6 and AT-7 gain the tests that forty source
comments have been citing — atomicity under rollback, channel parity across REST, MCP and
automation, and readability after the actor's account is deleted; `GET /audit` filters on period,
action, actor, target and outcome, and gains the `target` filter §5 requires and the specification
lacks; an export is a signed JSON Lines or CSV archive with a checksum manifest and a stated period,
and produces its own audit entry; the `AUDITOR` role reads the trail and no content, proved by tests
that try; new partitions carry their RLS policy and their revoked grants, proved by addressing a
freshly created partition directly as the app role; `docs/audit/event-matrix.md` is generated and
`gate-docs` is green; the evidence identifiers have one prefix and `audit.md` records the mapping.

**Read:** `audit.md` §2, §3, §4, §5, §8, §9; ADR-0017; `security.md` §5; `multi-tenancy.md` §2.2;
A-04 in `milestone-0.1.0.md`

---

## E-10 — Data subject requests: the case, its deadline, and its export **[L]**

*Depends on: E-04, E-07, E-08, E-09. The task that consumes the other three halves.*

`CreateDataSubjectRequest`, `UpdateDataSubjectRequest`, `ListDataSubjectRequests`,
`RestrictProcessing` and `WithdrawConsent`. `data_subject_request` has stood complete since
`0001_init` — the six kinds, the state machine `RECEIVED → IN_PROGRESS → COMPLETED | REJECTED`, the
statutory deadline, the assignee, the rejection reason, the export reference, and a partial index
built for exactly the deadline query — and no line of Go has ever touched it. Neither has
`consent_record`, nor `privacy_incident`. There is no `privacy` tag in the specification to hang
these on, so the tag and the paths are the specification-first step, and the use cases are new
entries in the `domain-model.md` §5 catalogue, which carries none of them.

`data-protection.md` §4 is the mapping and it is not one shape for six kinds: access and
portability produce an export; erasure is two-stage with the controller choosing anonymisation or
full deletion, because tenant data touches third parties' rights; restriction is a technical state
that keeps automation and AI away from a record; objection withdraws consent for optional
processing while the core features keep working; and rectification is an ordinary write that needs
no special path at all. The schema allows a `RECTIFICATION` kind anyway — as a tracked case with a
deadline it is coherent, and the task says which reading it implements rather than leaving a CHECK
constraint to imply one.

The export is not a new format. `backup-restore.md` §9 already settled it: an access or portability
export is a Hubtask archive, unencrypted or passphrase-protected, "so an export is therefore
simultaneously a restorable backup, without a second format coming into existence." That is why
this task depends on E-04 and not on an exporter of its own. Its own difficulty is scope: §4
requires a copy of the person's data across **every** tenant of the installation in which they are
a member, and this is the one operation in the system that legitimately crosses the tenant boundary.
It does so through an explicit, audited, instance-level path with its own permission — never by
relaxing `SET LOCAL app.tenant_id`, and never through a repository method that takes a tenant as an
argument (rule 3 does not bend for this).

Erasure is where four documents meet. Every storage location in the data catalogue is served —
rows, media, search index, derived counters — with an orphan check afterwards; the audit is exempt
and pseudonymises instead (the rule E-09 writes down); the `deletion_journal` takes its entries
under the reason `DSR_ERASURE`, a value the schema has allowed since phase 0 and nothing has ever
written, so that a restore does not bring the person back; and the backup retention period is the
effective upper bound on the deletion, documented in the catalogue and made transparent to the data
subject rather than concealed. QS-19 is exactly this chain, and it is the acceptance criterion.

Deadline tracking is the reason the feature is not merely a form: the alert fires as the statutory
period approaches, because "without deadline monitoring, the right gets violated in practice even
though the feature exists." Its identifier is settled with E-05 rather than beside it (decision in
E-05's text), and `data-retention.md` §4.2's wording gets corrected here — it calls `RESTRICTION` a
status where the schema makes it a kind, and an implementation that follows the document as written
would look for a value that cannot occur.

**Acceptance:** QS-19 demonstrated end to end — a request with a deadline, every storage location
from the catalogue served, audit references pseudonymised, the deletion journal written with
`DSR_ERASURE`, and a restore from an older archive proving the person does not return; an access
export is a Hubtask archive that E-06 can restore, covering every field the catalogue classifies as
personal; a restricted record is excluded from automation and from AI and is still readable; the
cross-tenant collection for an access request runs through the audited instance-level path with a
test proving no repository method gained a tenant argument; the state machine refuses an
illegitimate transition with a message code; the deadline alert fires against a request approaching
its due date and has a runbook; every new use case is in the `domain-model.md` §5 catalogue,
registered, and audited under a `dsr.*` action with `legal_basis` set — the field `audit.md` §2 has
carried for this since phase 0.

**Read:** `data-protection.md` §4, §5, §10, §12; ADR-0018; arc42 §8.15 and QS-19;
`backup-restore.md` §7, §9; `audit.md` §4; `multi-tenancy.md` §2

---

## E-11 — The privacy gates the documents have been promising **[G]**

*Depends on: E-07, E-10. Nothing can be verified before the thing it verifies exists.*

PG-1 to PG-8, and the reconciliation that has to happen before three of them can be written. Four
documents assert these gates as binding — `data-protection.md` §10, ADR-0018, the catalogue's own
header, and `versioning-release.md`'s release table — and they exist in no form at all: not a
Makefile target, not a script, not a test. The strings appear only in prose. C-09's acceptance
already claimed "PG-7 stays green" against nothing, which is precisely the failure mode the review
notice was written for: a green that was never a check.

The reconciliation comes first because the gates cannot be written without it. There are three
classification vocabularies in the repository and they are not the same: `data-protection.md` §3
has six classes; the catalogue's legend has five, omitting `SPECIAL_CATEGORY_RISK`; and `audit.md`
§4 masks by `OPEN`, `SENSITIVE`, `SECRET`. Two of those describe the same property at different
granularities and one is a masking policy derived from it. The task states the relationship — one
classification, one derivation rule to the masking vocabulary — and amends the documents so the
gate has a single thing to check against.

Then the gates, in the order of what they protect. PG-7 reconciles the schema against the catalogue
and is the one that stops the drift: every table and column holding personal content is recorded,
and the catalogue's own rule 6 constrains the implementation — a partition is not a data category,
so `audit_log_2026_08` resolves to its parent rather than demanding a row.
`schema_reference_test.go` is the precedent for how a document is checked against a live database
rather than trusted. PG-2 is the deletion test across every storage location after an erasure,
which ADR-0018 calls the
expensive one with the highest protective value; it belongs to the nightly with test containers,
and it is only writable because E-10 exists. PG-1 fails a build on an unclassified field with
personal content, PG-3 reconciles the catalogue against the access export's schema, PG-4 keeps
`PERSONAL_CONTENT` out of logs, metrics, traces, audit `changes` and error responses, PG-5 covers
the retention bounds E-07 built, PG-6 proves no outbound connection happens without configuration,
and PG-8 refuses third-country AI without confirmation — the last one has nothing to gate yet and
either lands as a test against the configuration surface or is recorded as belonging to `0.7.0`,
named rather than quietly absent.

Two promised documents are written here as well, because both are referenced by documents that
already ship: `docs/privacy/tom.md`, which `data-protection.md` §8 says is derived from
`security.md`, and the runbook `RB-GDPR-33` it names for a personal data breach.

**Acceptance:** PG-1 through PG-8 exist as runnable checks, each wired into the gate that suits its
cost — the cheap ones in `make verify`, PG-2 and PG-7 in the nightly with containers — and each one
is proved to go red by `gate-selftest` against a deliberate violation, which is what distinguishes
this task from the four documents that already claimed them; the three classification vocabularies
are reconciled in the documents and there is one source the gate reads; a new table with personal
content and no catalogue row fails a build, and a partition does not; the deletion test leaves no
orphan in database, object storage, search index, outbox, rule runs or deliveries, and the permitted
audit metadata is named rather than assumed; `docs/privacy/tom.md` and `RB-GDPR-33` exist and
`gate-docs` is green; `versioning-release.md`'s data protection row names checks that now run.

**Read:** `data-protection.md` §3, §5, §10; `docs/privacy/data-catalog.md` §1, §7; ADR-0018;
`ci-cd.md` §5; `engineering-guidelines.md` §3

---

## E-12 — hubctl grows with the milestone **[G]**

*Depends on: E-01 … E-11. The last task.*

B-13 built the CLI as the dogfooding client, C-13 grew it through 0.3.0 and D-09 through 0.4.0; the
same applies here: `hubctl backup target add/ls/test`, `backup run/ls/verify`, `hubctl restore
ls/inspect/run`, `hubctl retention ls/preview/retain`, `hubctl hold place/ls/release`, `hubctl audit
query/export/verify`, `hubctl dsr create/ls/complete`, and `hubctl job show/cancel` behind the
resource E-01 opened. The client types are generated from `openapi.yaml`, so most of the work is
commands rather than plumbing — a new file per group and a line in `groups()`.

The one that earns more than convenience is the restore drill, and it earns it twice. A scripted
`hubctl backup run` against a local target followed by `hubctl restore run --mode NEW_TENANT`
against the archive it produced, with the result compared to the source, is the first proof outside
a test that the whole chain — schedule, job, encryption, target, manifest, listing, restore —
survives a real round trip. It is also the drill A-19 measures the age of, so the CLI is what makes
that alert answerable rather than decorative. `backup-restore.md` says why in one line: a backup
that has never been restored is a hypothesis.

**Acceptance:** the scripted end-to-end session in CI grows the milestone's verbs — configure a
target, run a backup, list it at the target, verify it, restore it into a new tenant and compare,
preview a retention rule, take an object out of one, place and release a hold, query the audit and
verify its chain, raise and complete a data subject request — and stays green against the Compose
stack; the restore comparison is an assertion rather than an eyeball; `--json` stays pipeable for
every new command; errors render through the message-code catalogue rather than raw problem JSON;
a long-running command follows `/jobs/{id}` to completion and reports progress; the support matrix
rows still pass on every platform B-15 declared.

**Read:** `api-guidelines.md`; B-13 in `milestone-0.2.0.md`; C-13 in `milestone-0.3.0.md`; D-09 in
`milestone-0.4.0.md`; `support-matrix.md`

---

## The order at a glance

```
E-01 ─┬─ E-05  (+ E-04) ── E-06 ─────────────┐
      └─ E-09 ────────────────────┐          │
E-02 ── E-03 ─┬─ E-04 ────────────┤          │
              └─ E-07 ── E-08 ────┴─ E-10 ── E-11 ─┴─ E-12
```

E-01 and E-02 depend on nothing and start at once; E-09 hangs off E-01 alone and can run alongside
the whole backup chain. A task written with `(+ …)` needs those as well as the one it hangs from.
E-10 is where the three halves meet — it needs the archive from E-04, the safeguards from E-07 and
E-08, and the pseudonymisation rule from E-09 — and E-11 verifies what E-10 built. E-12 comes last:
it consumes every channel the others opened.

**Definition of Done for the milestone:** the backup, restore, retention, audit and privacy
sections of the use case catalogue are implemented for the 0.4.5 scope, each through REST, MCP and
automation with the full gate suite green; BK-1…BK-10, RE-1…RE-9, the audit evidence catalogue
under its single prefix, and PG-1…PG-8 all run and each is proved to go red by `gate-selftest`;
`test/backup/` and `test/audit/` hold tests rather than a `.gitkeep`, and `gate-data` earns the help
line it has carried since phase 0; alert A-12 has a metric behind it, A-19 has one identifier and a
runbook, and the first restore drill is recorded under `docs/evidence/` the way the resilience
evidence already is; an operator can configure a target, watch a backup run on a schedule, list it
without a database, restore it into a new tenant and compare the result; a tenant can preview a
retention rule before it deletes anything and stop it once it has started; an auditor can read,
export and verify the trail without being made an administrator; and a person who asks what is held
about them is answered inside a deadline the system watches, with the deletion reaching every
storage location the catalogue names and not returning on the next restore.
