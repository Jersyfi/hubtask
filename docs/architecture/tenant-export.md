# The tenant export format

Binding for the archive `POST /admin/tenants/{tenantId}:export` produces. Complements
[backup-restore.md](./backup-restore.md) §3 and §9, [multi-tenancy.md](./multi-tenancy.md) §5/§6,
and [security.md](./security.md) T-20.

"No lock-in" is a promise about a *format*: somebody must be able to build an importer against this
document and the two files it names (§2) without reading Hubtask's source. Where a rule lives in
code, the code follows this document.

---

## 1. One format, not two

A tenant export **is a Hubtask archive** — the format a backup writes, so an export is also a
restorable backup ([backup-restore.md](./backup-restore.md) §9). Circumstance distinguishes them,
never shape:

| | Backup run | Tenant export |
|---|---|---|
| Mode | `FULL` or `INCREMENTAL` | Always `FULL` |
| Encryption | The target's configuration | **Always `NONE`** — the receiver reads it without this installation's keys |
| Media | `include_media` flag | Always included |
| Audit entries | `include_audit` flag | Always included — a workspace's trail is part of what it owns |
| Name at the target | `hubtask-backup-<tenant>-<UTC>-full\|incremental` | `hubtask-export-<tenant>-<UTC>` |
| Asked for by | The workspace (schedule or `POST /backups`) | The control plane (`admin:tenants`) |

The distinct prefix keeps an export out of the backup **generation pruning**, which deletes under
`hubtask-backup-<tenant>-`, and out of the **restore listing** of the workspace's own backups. An
export is still importable: a restore names its archive by path (`source_archive`), whatever the
prefix.

The export works for `ACTIVE`, `SUSPENDED` and `PENDING_DELETION` workspaces alike — the suspended
and the leaving are exactly who needs it (multi-tenancy.md §5).

## 2. The documented surface

Three files together are the contract an importer builds against:

1. **This document** — the container, the manifest, the record line, the ordering rules, the media
   addressing, and the verification procedure.
2. **`db/schema.sql`** — the field dictionary. A record's `data` carries the columns of the entity's
   table (§5) under their column names; the manifest's `schema_version` names the migration state
   the archive was written under.
3. **`docs/privacy/data-catalog.md`** — which of those fields are personal data.

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

* **`checksums.txt` is the commit point.** An archive without it is incomplete — still being
  written, or a run that died — and must not be read as an archive.
* Every other member is listed in `checksums.txt` as `<sha256-lower-hex>  <path>` (two spaces), one
  line per member, the digest over the member's bytes as stored. A tenant export is never
  encrypted, so those are the plaintext; at an encrypted backup target they are the ciphertext, and
  the manifest's `encryption` says which.
* An entity with no rows still has its `data/<entity>.jsonl` — empty, listed in the manifest with a
  count of 0. A missing data file is a defect, except `data/audit.jsonl`, which a backup may
  exclude; a tenant export always writes it.

## 4. The manifest

`manifest.json` is an indented JSON object. Fields, all at the top level:

| Field | Type | Meaning |
|---|---|---|
| `format_version` | int | This document describes **version 1**. Readers must refuse a version above the one they know. |
| `archive_id` | uuid | The archive's own identity; for an export, also the export job's subject in the audit trail. |
| `schema_version` | string | The database migration state the writer ran under (§2). |
| `product_version` | string | The Hubtask build that wrote the archive. |
| `mode` | string | `FULL` for every tenant export. (An `INCREMENTAL` backup names its `parent_id`/`parent_prefix`.) |
| `scope` | object | `{"kind": "TENANT", "id": "<tenant-uuid>"}` — whose data this is. |
| `period` | object | `{"to": <RFC 3339>}` — the snapshot moment; `from` appears only on incrementals. |
| `snapshot_at` | RFC 3339 | The consistent read moment: every data file describes the database at this one instant (one `REPEATABLE READ` snapshot). |
| `encryption` | object | `{"mode": "NONE"}` for every tenant export. |
| `counts` | object | Entity name → number of records in its data file — **what a verifier compares against the line counts and the source database**. |
| `media_count` | int | How many objects sit under `media/`. |
| `media_bytes` | int | Their summed size. Media are counted, never listed. |
| `whole` | array | The entity names whose file is always the complete set (meaningful for incrementals). |
| `files` | array | One entry per data member: `{"path", "bytes", "sha256", "records"}`; the digest equals the one in `checksums.txt`. |

The manifest carries **no user content** — counts by entity, never names — because it is the one
member never encrypted in any archive, so a target listing can be read without keys.

## 5. The record line

Each line of a `data/<entity>.jsonl` file is one JSON object:

| Field | Type | Meaning |
|---|---|---|
| `id` | string | The row's identity: the entity's key columns (§6), joined with `/` when there are several. Never contains the tenant — the archive's scope carries it once. |
| `op` | string | `UPSERT` — the row as it stood at the snapshot. (`DELETE` markers exist only in incremental backups.) |
| `updated_at` | RFC 3339 | When the row last changed. Always present; a table with no change stamp gives the archive's `snapshot_at`. |
| `data` | object | **The row's columns, under their column names, minus `tenant_id`** (`db/schema.sql` at the manifest's `schema_version`). Values as JSON: text as strings, numerics as numbers, `timestamptz` as RFC 3339 strings, `jsonb` embedded as-is, arrays as arrays, `NULL` as `null`. |
| `blobs` | array | Only on `media_objects` records whose bytes are in the archive: `[{"sha256": "<64 lower hex>", "bytes": <int>}]` — the reference into `media/` (§7). |

A line is at most 4 MiB. A record whose bytes were already gone at export time is still written,
with no `blobs` entry, so an importer knows the attachment existed and that its content is gone.

## 6. The entities

The data files, **in the order they appear and must be applied**: a row's parents come before it,
with the three exceptions below the table. *Identity* is the `id` field's composition;
*references* are the fields in `data` that point at another entity's `id` (an importer that
re-mints identities must rewrite exactly these).

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
| 31 | `audit` | `audit_log` | `seq` | — (last, and read-only on import: an insert into the middle of a hash chain is a rewrite, not a restore) |

Cross-references between two rows of the same file (a collection's hub, a subtask's parent, a
reply's comment) are ordered within the file: the referenced row comes first.

**Three references point forward.** In `work_items`, `cover_media_id` (#15), `recurrence_rule_id`
(#17) and `origin_jumble_id` (#21) name rows of later files; an importer that enforces them sets
them after the later file is applied. `work_items` also carries `recurrence_source_id`, a reference
to another `work_items` row without a foreign key. Of the three, only `cover_media_id` is a foreign
key in Hubtask's own schema, and its restore applies `media_objects`, which references nothing,
before `work_items` instead (and empties a replaced workspace in the reverse of that order).

## 7. Media

File bytes are stored **content-addressed** at
`media/<first two hex digits of its SHA-256>/<full SHA-256, lower hex>`; the fan-out keeps
directories small.

* A `media_objects` record whose bytes are in the archive carries a `blobs` entry naming the
  digest and size; the digest **is** the path. It is the live system's own content checksum, never
  recomputed at export time.
* Objects are deduplicated: two attachments with the same bytes are one object. The manifest counts
  objects, not references.
* A record without `blobs` is an attachment whose content was already gone (or never sealed) at
  export time.

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
5. **Scope** (T-20): every record belongs to the workspace the manifest's `scope` names. No record
   carries a `tenant_id`, so the archive has exactly one workspace by construction, and the writer
   reaches rows only through the row-level-security path the API uses. A test writes an archive in
   a two-tenant installation and searches the bytes for the other tenant's identifiers.

## 9. What is deliberately absent

An export contains a workspace's **data**, never its **credentials or live machinery** (the
reasoning is backup-restore.md §8.4):

* **Credentials, whole and half**: access tokens, sessions and refresh chains, sign-in attempts,
  second-factor enrolments and recovery codes, password history, pending sign-ins and provider
  flows, OAuth clients/grants/codes, sign-in provider configurations and linked external
  identities, the workspace's host names, calendar feed token hashes, the jumble intake credential,
  device registrations. A copy of a credential is a credential.
* **Live plumbing**: the outbox and event consumptions, notification rows, webhook deliveries,
  rule runs and occurrences, idempotency keys, the synchronisation's change log, operation log,
  field clocks and tombstones, usage records.
* **AI machinery**: the AI provider configuration (with its sealed key), suggestions, pending AI
  requests, and embedding vectors (derived from content that is in the export).
* **The compliance machinery's own state**: data subject requests, privacy incidents, the deletion
  journal, audit anchors and pseudonyms, backup targets and schedules, backup/restore/import/
  retention run bookkeeping, retention rules. Cases and attestations belong to the installation
  that handled them.
* **The product's own shape**: the system capability profiles.

The list is `ExcludedTables()` in `core/application/archive/Record.go`; a test holds every table under
row level security to being either an entity of §6 or on that list.

Where an included table mixes data with a credential, the export **redacts the column**: the field
is absent from the record, not null. The redacted fields, exhaustively:

| Entity | Absent fields |
|---|---|
| `accounts` | `password_hash`, `redemption_token_hash` |
| `automation_rules` | `inbound_token_hash` |
| `webhook_subscriptions` | `secret_enc`, `secret_key_id`, `previous_secret_enc`, `previous_secret_key_id`, `previous_secret_until` |
| `calendar_feeds` | `token_hash` |

A *backup* keeps these columns: it is encrypted at the operator's own target, and a restore is
expected to keep sign-ins working. The export is handed outwards.

## 10. Importing

* **Into a Hubtask installation**: a restore naming the archive's path as `source_archive` with mode
  `NEW_TENANT` (backup-restore.md §8) — the provider-migration path. The new installation's
  operator runs it, and it is accepted because no workspace there bears the manifest's identifier.
  Decided, not built: the restore refuses an archive of another workspace today (#1074).
* **Into anything else**: apply the data files in §6's order; treat `id` as the row identity and
  the references as foreign keys; fetch media by digest. An importer that only wants the content
  can stop after `work_items`, `comments` and `media/`.

## 11. Versioning

* `format_version` is currently **1**, shared with the backup archive
  ([versioning-release.md](./versioning-release.md)): additive fields do not bump it, a change a
  version-1 reader would misread does.
* Readers refuse a `format_version` above what they know, and accept everything from the minimum
  readable version (currently 1) up.
* This document is versioned with the repository; the manifest's `product_version` and
  `schema_version` say which state of it applied when an archive was written.
