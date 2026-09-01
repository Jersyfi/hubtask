# The tenant export format

Binding for the archive `POST /admin/tenants/{tenantId}:export` produces (H-07). Complements
[backup-restore.md](./backup-restore.md) §3 and §9, [multi-tenancy.md](./multi-tenancy.md) §5/§6,
and [security.md](./security.md) T-20.

This document is half of the deliverable. "No lock-in" is a promise about a *format*, not about a
feature: somebody must be able to build an importer against what is written here — this file, plus
the two files it names as part of its surface — without reading Hubtask's source. Everything an
importer needs is specified below; where a rule lives in code, the code follows this document, not
the other way round.

---

## 1. One format, not two

A tenant export **is a Hubtask archive** — the same format a backup writes
([backup-restore.md](./backup-restore.md) §9: "an export is therefore simultaneously a restorable
backup, without a second format coming into existence"). What distinguishes an export from a backup
is circumstance, never shape:

| | Backup run | Tenant export |
|---|---|---|
| Mode | `FULL` or `INCREMENTAL` | Always `FULL` — one archive is the whole answer |
| Encryption | The target's configuration | **Always `NONE`** — the receiver reads it without this installation's keys, which is the point |
| Media | `include_media` flag | Always included |
| Audit entries | `include_audit` flag | Always included — a workspace's trail is part of what it owns |
| Name at the target | `hubtask-backup-<tenant>-<UTC>-full\|incremental` | `hubtask-export-<tenant>-<UTC>` |
| Asked for by | The workspace (schedule or `POST /backups`) | The control plane (`admin:tenants`) |

The distinct name prefix is deliberate, twice over: the backup **generation pruning** counts and
deletes everything under `hubtask-backup-<tenant>-`, and an export must not be pruned as though it
were a generation; and the **restore listing** shows the same prefix, and an export is not one of
the workspace's own backups. An export is still importable — a restore names its archive by path
(`source_archive`), and any path at the target works, whatever its prefix.

The export works for `ACTIVE`, `SUSPENDED` and `PENDING_DELETION` workspaces alike. The suspended
and the leaving are exactly who needs it (multi-tenancy.md §5).

## 2. The documented surface

Three files together are the contract an importer builds against:

1. **This document** — the container, the manifest, the record line, the ordering rules, the media
   addressing, and the verification procedure.
2. **`db/schema.sql`** — the field dictionary. A record's `data` object carries the columns of the
   entity's table (§5), under their column names, and `db/schema.sql` is the readable, versioned
   reference for what each column means. The manifest's `schema_version` names the migration state
   the archive was written under.
3. **`docs/privacy/data-catalog.md`** — which of those fields are personal data, for an importer
   that has to answer the same questions this system does.

## 3. The container

An archive is a directory of objects under one name (the *prefix*) at a storage target:

```
hubtask-export-<tenant-uuid>-<YYYYMMDD>T<HHMMSS>Z/
├── manifest.json           what the archive is (§4); plain JSON, never encrypted
├── data/<entity>.jsonl     one file per entity (§6), one record per line (§5), UTF-8
├── media/<xx>/<sha256>     file bytes, content-addressed (§7); <xx> = first two hex digits
└── checksums.txt           SHA-256 per member — written LAST, the commit point (§8)
```

Rules an importer may rely on:

* **`checksums.txt` is the commit point.** An archive without it is incomplete — a run that is
  still writing, or one that died — and must not be read as an archive.
* Every member except `checksums.txt` is listed in `checksums.txt` as
  `<sha256-lower-hex>  <path>` (two spaces), one line per member, and the digest is over the
  member's bytes exactly as stored.
* A tenant export is never encrypted, so the stored bytes are the plaintext. (A backup at an
  encrypted target stores ciphertext, and its digests are over the ciphertext; the manifest's
  `encryption` object says which case an archive is.)
* An entity with no rows still has its `data/<entity>.jsonl` — empty, zero records, listed in the
  manifest with a count of 0. Absence of a data file is a defect, with one exception:
  `data/audit.jsonl` is optional in the general format (a backup may exclude it); a tenant export
  always writes it.

## 4. The manifest

`manifest.json` is an indented JSON object. Fields, all at the top level:

| Field | Type | Meaning |
|---|---|---|
| `format_version` | int | This document describes **version 1**. Readers must refuse a version above the one they know. |
| `archive_id` | uuid | The archive's own identity; for an export, also the export job's subject in the audit trail. |
| `schema_version` | string | The database migration state the writer ran under (the field dictionary's version, §2). |
| `product_version` | string | The Hubtask build that wrote the archive. |
| `mode` | string | `FULL` for every tenant export. (`INCREMENTAL` exists for backups; an incremental archive names its `parent_id`/`parent_prefix` and is not produced by the export.) |
| `scope` | object | `{"kind": "TENANT", "id": "<tenant-uuid>"}` — whose data this is. |
| `period` | object | `{"to": <RFC 3339>}` — the snapshot moment; `from` appears only on incrementals. |
| `snapshot_at` | RFC 3339 | The consistent read moment: every data file describes the database as it stood at this one instant (one `REPEATABLE READ` snapshot). |
| `encryption` | object | `{"mode": "NONE"}` for every tenant export. |
| `counts` | object | Entity name → number of records in its data file. **These are the numbers a verifier compares against the line counts, and against the source database.** |
| `media_count` | int | How many objects sit under `media/`. |
| `media_bytes` | int | Their summed size. Media are counted, never listed — a digest list would be the largest member of a large archive. |
| `whole` | array | The entity names whose file is always the complete set (meaningful for incrementals; informational on a `FULL` archive, where every file is complete). |
| `files` | array | One entry per data member: `{"path", "bytes", "sha256", "records"}`. The digest here equals the one in `checksums.txt`. |

The manifest deliberately carries **no user content** — counts by entity, never names. It is the
one member that is never encrypted in any archive, so that a target listing can be read without
keys.

## 5. The record line

Each line of a `data/<entity>.jsonl` file is one JSON object:

| Field | Type | Meaning |
|---|---|---|
| `id` | string | The row's identity: the entity's key columns (§6), joined with `/` when there are several. Never contains the tenant — the archive's scope carries it once. |
| `op` | string | `UPSERT` — the row as it stood at the snapshot. (`DELETE` markers exist only in incremental backups; a `FULL` export contains none.) |
| `updated_at` | RFC 3339 | When the row last changed. Always present; for entities whose table records no change stamp it is the zero instant `0001-01-01T00:00:00Z`. |
| `data` | object | **The row's columns, under their column names, minus `tenant_id`** — the field dictionary is `db/schema.sql` at the manifest's `schema_version`. Values are the column values as JSON: text as strings, numerics as numbers, `timestamptz` as RFC 3339 strings, `jsonb` embedded as-is, arrays as arrays, `NULL` as `null`. |
| `blobs` | array | Only on `media_objects` records whose bytes are in the archive: `[{"sha256": "<64 lower hex>", "bytes": <int>}]` — the reference into `media/` (§7). |

A line is at most 4 MiB. A record whose `data` lost its meaning (an attachment whose bytes were
already gone at export time) is still written — with no `blobs` entry — so an importer knows the
attachment existed and that its content is gone.

## 6. The entities

The data files, **in the order they appear and must be applied**: a row's parents come before it,
so an importer can apply the files in sequence without deferring references. `identity` is the
`id` field's composition; `references` are the fields in `data` that point at another entity's
`id` (an importer that re-mints identities must rewrite exactly these).

| # | Entity | Table | Identity | References (field → entity) |
|---|---|---|---|---|
| 1 | `tenants` | `tenant` | `id` | — (the workspace itself: slug, display name, defaults, settings) |
| 2 | `accounts` | `account` | `id` | — (people and service accounts; credentials are **not** among the fields, §9) |
| 3 | `account_groups` | `account_group` | `id` | — |
| 4 | `account_group_members` | `account_group_member` | `group_id/account_id` | `group_id`→account_groups, `account_id`→accounts |
| 5 | `memberships` | `membership` | `id` | `account_id`→accounts, `group_id`→account_groups |
| 6 | `containers` | `container` | `id` | `parent_id`→containers (hubs first — a parent precedes its children within the file) |
| 7 | `buckets` | `bucket` | `id` | `collection_id`→containers |
| 8 | `labels` | `label` | `id` | `collection_id`→containers |
| 9 | `custom_field_definitions` | `custom_field_definition` | `id` | `collection_id`→containers |
| 10 | `work_items` | `work_item` | `id` | `collection_id`→containers, `parent_id`→work_items, `bucket_id`→buckets, `assignee_id`→accounts, `cover_media_id`→media_objects, `recurrence_rule_id`→recurrence_rules, `origin_jumble_id`→jumble_entries |
| 11 | `item_labels` | `item_label` | `item_id/label_id` | `item_id`→work_items, `label_id`→labels |
| 12 | `item_members` | `item_member` | `item_id/account_id` | `item_id`→work_items, `account_id`→accounts |
| 13 | `comments` | `comment` | `id` | `item_id`→work_items, `parent_comment_id`→comments |
| 14 | `activity_entries` | `activity_entry` | `id` | `item_id`→work_items |
| 15 | `media_objects` | `media_object` | `id` | — (`blobs` points into `media/`, §7) |
| 16 | `item_attachments` | `item_attachment` | `item_id/media_id` | `item_id`→work_items, `media_id`→media_objects |
| 17 | `recurrence_rules` | `recurrence_rule` | `id` | `source_item_id`→work_items |
| 18 | `reminders` | `reminder` | `id` | `item_id`→work_items |
| 19 | `saved_views` | `saved_view` | `id` | — |
| 20 | `templates` | `template` | `id` | — |
| 21 | `jumble_entries` | `jumble_entry` | `id` | — |
| 22 | `auto_assign_policies` | `auto_assign_policy` | `id` | — |
| 23 | `automation_rules` | `automation_rule` | `id` | `run_as`→accounts |
| 24 | `webhook_subscriptions` | `webhook_subscription` | `id` | — |
| 25 | `calendar_feeds` | `calendar_feed` | `id` | `account_id`→accounts, `view_id`→saved_views (the feed's token is not among the fields, §9) |
| 26 | `notification_preferences` | `notification_preference` | `account_id/category/channel` | `account_id`→accounts |
| 27 | `retention_policies` | `retention_policy` | `data_kind` | — |
| 28 | `consent_records` | `consent_record` | `id` | `account_id`→accounts |
| 29 | `legal_holds` | `legal_hold` | `id` | — |
| 30 | `set_elements` | `set_element` | `item_id/set_name/element_id` | `item_id`→work_items |
| 31 | `audit` | `audit_log` | `seq` | — (last, and read-only on import: the trail is a hash chain, and an insert into the middle of one is a rewrite, not a restore) |

Cross-references between two rows of the same file (a collection's hub, a subtask's parent, a
reply's comment) are ordered within the file: the referenced row's line comes first.

## 7. Media

File bytes are stored **content-addressed**: an object's path is
`media/<first two hex digits of its SHA-256>/<full SHA-256, lower hex>`. The two-level fan-out
keeps directories small on filesystems and WebDAV servers that dislike wide ones.

* A `media_objects` record whose bytes are in the archive carries a `blobs` entry naming the
  digest and size; the digest **is** the path. The digest comes from the live system's own
  content checksum and is never recomputed at export time.
* Objects are deduplicated: two attachments with the same bytes are one object under `media/`.
  The manifest counts objects, not references.
* A record without `blobs` is an attachment whose content was already gone (or never sealed) at
  export time — the row survives as the statement that it existed.

## 8. Verifying an archive

A verifier — human or importer — checks, in this order:

1. **Completeness**: `checksums.txt` exists. Without it, stop: the archive is not committed.
2. **Integrity**: every member listed in `checksums.txt` exists and its SHA-256 matches; no data
   member exists that is unlisted.
3. **Consistency**: `manifest.json`'s `files[].sha256` agree with `checksums.txt`; each data
   file's line count equals its `files[].records` and the manifest's `counts[<entity>]`.
4. **Media**: every `blobs` digest resolves to an object under `media/`, the object's SHA-256
   equals its own path, and its size equals `blobs[].bytes`; `media_count`/`media_bytes` match
   what is actually there.
5. **Scope** (T-20): every record belongs to the workspace the manifest's `scope` names. The
   format gives this teeth by *omission* — no record carries a `tenant_id`, so the archive has
   exactly one workspace by construction, and the writer reaches rows only through the same
   row-level-security path the API uses. The test suite proves it by writing an archive in a
   two-tenant installation and searching the bytes for the other tenant's identifiers.

## 9. What is deliberately absent

An export contains a workspace's **data**, never its **credentials or live machinery**. Absent by
design, with the reasoning of backup-restore.md §8.4:

* **Credentials, whole and half**: access tokens, sessions and their refresh chains, TOTP
  enrolments and recovery codes, pending sign-ins, OAuth clients/grants/codes, calendar feed
  token hashes, intake token hashes, device registrations. A copy of a credential is a credential.
* **Live plumbing**: the outbox and event consumptions, notification rows, webhook deliveries,
  rule runs and occurrences, idempotency keys, sync cursors and tombstone markers, usage records.
  These describe the machine's moment, not the workspace's content.
* **The compliance machinery's own state**: data subject requests, audit anchors and pseudonyms,
  backup/restore/retention run bookkeeping, retention rules' operator configuration. Cases and
  attestations belong to the installation that handled them.

Where an included table mixes data with a credential, the export **redacts the column**: the
field is absent from the record, not null. The redacted fields, exhaustively:

| Entity | Absent fields |
|---|---|
| `accounts` | `password_hash`, `redemption_token_hash` |
| `automation_rules` | `inbound_token_hash` |
| `webhook_subscriptions` | `secret_enc`, `secret_key_id`, `previous_secret_enc`, `previous_secret_key_id`, `previous_secret_until` |
| `calendar_feeds` | `token_hash` |

(A *backup* keeps these columns — it is encrypted at the operator's own target and a restore is
expected to keep sign-ins working. The export is handed outwards, which is why it does not.)

## 10. Importing

Two paths:

* **Into a Hubtask installation**: a restore that names the archive's path as `source_archive`
  with mode `NEW_TENANT` (backup-restore.md §8) — this is the provider-migration path, and the
  reason the export and the backup share one format.
* **Into anything else**: apply the data files in §6's order; treat `id` as the row identity and
  the references as foreign keys; fetch media by digest as needed. An importer that only wants
  the content can stop after `work_items`, `comments` and `media/`.

## 11. Versioning

* `format_version` is currently **1**, shared with the backup archive
  ([versioning-release.md](./versioning-release.md)): additive fields do not bump it, a change a
  version-1 reader would misread does.
* Readers refuse a `format_version` above what they know, and accept everything from the minimum
  readable version (currently 1) up.
* This document is versioned with the repository; the manifest's `product_version` and
  `schema_version` say which state of it applied when an archive was written.
