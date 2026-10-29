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
| `data` | object | **The row's columns, under their column names, minus `tenant_id`** (`db/schema.sql` at the manifest's `schema_version`) and minus the columns §11 names as left out. Values as JSON: text as strings, numerics as numbers, `timestamptz` as RFC 3339 strings, `jsonb` embedded as-is, arrays as arrays, `NULL` as `null`. |
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
| 3 | `webauthn_credentials` | `webauthn_credential` | `id` | `account_id`→accounts (a person's passkeys; an export carries their name and dates, not the credential, §9) |
| 4 | `account_groups` | `account_group` | `id` | — |
| 5 | `account_group_members` | `account_group_member` | `group_id/account_id` | `group_id`→account_groups, `account_id`→accounts |
| 6 | `memberships` | `membership` | `id` | `account_id`→accounts, `group_id`→account_groups |
| 7 | `containers` | `container` | `id` | `parent_id`→containers (hubs first — a parent precedes its children within the file). `private` marks a private hub; an archive without it restores the hub shared, as it was |
| 8 | `buckets` | `bucket` | `id` | `collection_id`→containers |
| 9 | `labels` | `label` | `id` | `collection_id`→containers |
| 10 | `custom_field_definitions` | `custom_field_definition` | `id` | `collection_id`→containers |
| 11 | `entry_types` | `entry_type` | `id` | — (the workspace's own entry types and its rows for the system types it changed, migration 0209; a work item names one by `key` in `entry_type`, so they precede the work items; a duplicate skips a key the workspace holds) |
| 12 | `work_items` | `work_item` | `id` | `entry_type`→entry_types (by key), `collection_id`→containers, `parent_id`→work_items, `bucket_id`→buckets, `assignee_id`→accounts, `cover_media_id`→media_objects, `recurrence_rule_id`→recurrence_rules, `origin_jumble_id`→jumble_entries |
| 13 | `item_labels` | `item_label` | `item_id/label_id` | `item_id`→work_items, `label_id`→labels |
| 14 | `item_members` | `item_member` | `item_id/account_id` | `item_id`→work_items, `account_id`→accounts |
| 15 | `comments` | `comment` | `id` | `item_id`→work_items, `parent_comment_id`→comments |
| 16 | `activity_entries` | `activity_entry` | `id` | `item_id`→work_items |
| 17 | `media_objects` | `media_object` | `id` | — (`blobs` points into `media/`, §7) |
| 18 | `item_attachments` | `item_attachment` | `item_id/media_id` | `item_id`→work_items, `media_id`→media_objects |
| 19 | `recurrence_rules` | `recurrence_rule` | `id` | `source_item_id`→work_items |
| 20 | `reminders` | `reminder` | `id` | `item_id`→work_items |
| 21 | `saved_views` | `saved_view` | `id` | — |
| 22 | `templates` | `template` | `id` | — |
| 23 | `jumble_entries` | `jumble_entry` | `id` | — |
| 24 | `auto_assign_policies` | `auto_assign_policy` | `id` | — |
| 25 | `automation_rules` | `automation_rule` | `id` | `run_as`→accounts |
| 26 | `webhook_subscriptions` | `webhook_subscription` | `id` | — |
| 27 | `calendar_feeds` | `calendar_feed` | `id` | `account_id`→accounts, `view_id`→saved_views (the feed's token is not among the fields, §9) |
| 28 | `notification_preferences` | `notification_preference` | `account_id/category/channel` | `account_id`→accounts |
| 29 | `retention_policies` | `retention_policy` | `data_kind` | — |
| 30 | `consent_records` | `consent_record` | `id` | `account_id`→accounts |
| 31 | `legal_holds` | `legal_hold` | `id` | — |
| 32 | `private_hub_accesses` | `private_hub_access` | `id` | `hub_id`→containers, `opened_by`→accounts (an access whose hub is not restored is skipped) |
| 33 | `container_milestones` | `container_milestone` | `container_id` | `container_id`→containers (when a collection or hub was last finished; a milestone whose container is not restored is skipped) |
| 34 | `set_elements` | `set_element` | `item_id/set_name/element_id` | `item_id`→work_items |
| 35 | `structure_presets` | `structure_preset` | `id` | `owner_id`→accounts (the workspace's presets and each person's own; an own preset whose account is not restored is skipped) |
| 36 | `setting_values` | `setting_value` | `id` | — (the settings catalogue's entries of the workspace, its hubs, collections and people, the module switches among them; `scope_id` names the workspace, a container or an account by `level`, and a workspace's own entries follow the workspace written into) |
| 37 | `link_types` | `link_type` | `id` | — (the workspace's own link types, migration 0213; the six shipped ones have no row; a link names one by `key`, so they precede the links; a duplicate skips a key the workspace holds) |
| 38 | `entry_links` | `entry_link` | `id` | `source_id`→work_items, `target_id`→work_items, `created_by`→accounts (`link_type` names a shipped type or one of `link_types` by key) |
| 39 | `custom_roles` | `custom_role` | `id` | — (the workspace's own roles from the permission catalogue, migration 0219; restored right after `memberships`, before their grants; the seven built-in roles have no row) |
| 40 | `custom_role_grants` | `custom_role_grant` | `id` | `account_id`→accounts, `group_id`→account_groups, `role_id`→custom_roles (`scope_id` names the workspace's container as a membership's does) |
| 41 | `config_revisions` | `config_revision` | `version` | — (the configuration document's versions, migration 0221, a hub's with its `hub_id` since 0223; written once, so a version the workspace holds is kept) |
| 42 | `config_proposals` | `config_proposal` | `id` | — (agents' plans kept for a person, and how each was decided) |
| 43 | `config_templates` | `config_template` | `id` | — (the workspace's template versions; one the workspace holds by identity or by key and version is kept) |
| 44 | `config_template_uses` | `config_template_use` | `template_key` | — (the template version a workspace took, the base of a three-way upgrade) |
| 45 | `realtime_documents` | `realtime_document` | `document_id/kind` | — (a live document's head, migration 0230; `document_id` names what its `kind` belongs to - a page, a whiteboard - without a foreign key) |
| 46 | `realtime_updates` | `realtime_update` | `document_id/kind/seq` | `author_id`→accounts (the editor's CRDT updates as sent, `data` as hex; what retention removed after a snapshot is not there) |
| 47 | `realtime_snapshots` | `realtime_snapshot` | `document_id/kind/up_to_seq` | — (a writer's merged state up to an update, `state` as hex) |
| 48 | `pages` | `page` | `id` | `hub_id`→containers, `parent_id`→pages, `owner_id`→accounts (a hub's pages, migration 0231; `body` is Markdown with `hubtask:` references; the search document and the references are rebuilt on import; `parent_id` may point forward, a moved page coming before a newer parent, and the schema checks it at the commit) |
| 49 | `page_versions` | `page_version` | `page_id/seq` | `page_id`→pages, `author_id`→accounts (a page's history, one row per editing session) |
| 50 | `page_comments` | `page_comment` | `id` | `page_id`→pages, `parent_id`→page_comments, `author_id`→accounts |
| 51 | `whiteboards` | `whiteboard` | `id` | `hub_id`→containers (a hub's boards, migration 0233; `scene` is the board's objects as JSON in the contract's shape, a card naming its entry by identifier; where the board points is rebuilt on import) |
| 52 | `whiteboard_versions` | `whiteboard_version` | `whiteboard_id`, `seq` | `whiteboard_id`→whiteboards, `author_id`→accounts (cleared when absent) (a board's history, migration 0234: one version per drawing session, its scene as JSON in the contract's shape) |
| 53 | `whiteboard_media` | `whiteboard_media` | `whiteboard_id`, `media_id` | `whiteboard_id`→whiteboards, `media_id`→media_objects (the pictures and files placed on a board, migration 0235) |
| 54 | `cmdb_classes` | `cmdb_class` | `id` | — (the workspace's own configuration item classes, migration 0236; attributes, identifier rules and required relations as JSON) |
| 55 | `cmdb_relation_types` | `cmdb_relation_type` | `id` | — (the workspace's own relation types between CIs, with their impact and class pairs) |
| 56 | `cmdb_sources` | `cmdb_source` | `id` | — (the sources CIs come from, their rank, classes and a file source's mapping) |
| 57 | `cmdb_cis` | `cmdb_ci` | `id` | `hub_id`→containers, `owner_account_id`→accounts and `owner_group_id`→account_groups (cleared when absent) (a hub's configuration items: typed attributes, provenance per attribute, lifecycle and compliance fields; `location_ci_id` and `merged_into` name CIs without a foreign key) |
| 58 | `cmdb_ci_identifiers` | `cmdb_ci_identifier` | `ci_id`, `name` | `ci_id`→cmdb_cis (the identifier index the reconciliation looks up, and each source's own key) |
| 59 | `cmdb_relations` | `cmdb_relation` | `id` | `source_ci_id`, `target_ci_id`→cmdb_cis (relations between a hub's CIs with their source and first and last seen) |
| 60 | `cmdb_runs` | `cmdb_run` | `id` | `hub_id`→containers (the reconciliation's runs with their decisions) |
| 61 | `cmdb_holds` | `cmdb_hold` | `id` | `run_id`→cmdb_runs (items held for review, the item as reported) |
| 62 | `cmdb_duplicates` | `cmdb_duplicate` | `id` | `hub_id`→containers (duplicates opened to merge, with their survivor once resolved) |
| 63 | `cmdb_connectors` | `cmdb_connector` | `id` | `hub_id`→containers, `collection_id`→containers (cleared when absent), `run_as`→accounts (a hub's connectors with their settings, schedule, cursor and last run, migration 0238; the sealed credential is a backup's only) |
| 64 | `cmdb_alerts` | `cmdb_alert` | `id` | `connector_id`→cmdb_connectors (the alerts an intake took in, bound to a CI and an entry by identifier, without a foreign key) |
| 54 | `audit` | `audit_log` | `seq` | — (last, and read-only on import: an insert into the middle of a hash chain is a rewrite, not a restore) |

Cross-references between two rows of the same file (a collection's hub, a subtask's parent, a
reply's comment) are ordered within the file: the referenced row comes first.

**Three references point forward.** In `work_items`, `cover_media_id` (#17), `recurrence_rule_id`
(#19) and `origin_jumble_id` (#23) name rows of later files; an importer that enforces them sets
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
  device registrations, the digests of the CMDB's alert deliveries that refuse a replay
  (`cmdb_intake_seen`). A copy of a credential is a credential.
* **Live plumbing**: the outbox and event consumptions, notification rows, webhook deliveries,
  rule runs and occurrences, durable timers (migration 0229; a clock towards the source system's
  future), idempotency keys, the synchronisation's change log, operation log,
  field clocks and tombstones, usage records.
* **Projections rebuilt from the archive**: the typed custom field values (`entry_field_value`,
  migration 0211), which the restore's own write of each entry's `custom_fields` rebuilds - the
  definitions precede the work items, so every value finds the definition it was written under;
  and the references texts hold (`entry_reference`, migration 0213), which the restore's write of
  each entry's notes and each comment rebuilds; and the slim entry's side tables
  (`work_item_announcement`, `work_item_flow`, `work_item_retention`, migration 0227), mirrors of
  the entry's own columns that the restore's write of each entry rebuilds.
* **AI machinery**: the AI provider configuration (with its sealed key), the workspace's choice of AI
  source (choosing an offered model is a consent a restore must not resume), suggestions, pending AI
  requests, and embedding vectors (derived from content that is in the export). The workspace's own
  narrowing of which sources it may use is a restriction, and travels with the workspace's settings.
* **The compliance machinery's own state**: data subject requests, privacy incidents, the deletion
  journal, audit anchors and pseudonyms, backup targets and schedules, backup/restore/import/
  retention run bookkeeping, retention rules. Cases and attestations belong to the installation
  that handled them.
* **The product's own shape**: the system capability profiles.
* **Who acknowledged whom**: kudos are the business of the person who gave them and the person who
  received them (UC-WRK-29 check 6), and an export or a backup is an administrator's to read. The
  copy a person gets of their own data carries the kudos they gave and received as a section,
  `data/kudos.jsonl` (`item_id`, `given_by`, `given_to`, `given_at`;
  [data-protection.md](./data-protection.md) §4).

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
| `cmdb_connectors` | `credential_enc`, `credential_key_id` |
| `webauthn_credentials` | `credential_id`, `public_key`, `user_handle` — the passkey leaves by name and dates, and an import skips it ([identity.md](./identity.md) §23.5) |

A *backup* keeps these columns: it is encrypted at the operator's own target, and a restore is
expected to keep sign-ins working. The export is handed outwards.

## 10. Importing

* **Into a Hubtask installation**: a restore naming the archive's path as `source_archive` with mode
  `NEW_TENANT` (backup-restore.md §8) — the provider-migration path. The new installation's
  operator runs it, and it is accepted because no workspace there bears the manifest's identifier;
  the operator's listing at the target shows the export (backup-restore.md §8.1). The people sign in
  again and make new tokens: an export carries no credential (§9). A managed account
  ([identity.md](./identity.md) §13) arrives with its sign-in name and without a password, and has
  no mailbox to set one through: its way in is a new start password from an owner or
  administrator of the new workspace (identity.md §13; decided, not built).
* **Into anything else**: apply the data files in §6's order; treat `id` as the row identity and
  the references as foreign keys; fetch media by digest. An importer that only wants the content
  can stop after `work_items`, `comments` and `media/`.

## 11. Versioning

* `format_version` is currently **1**, shared with the backup archive
  ([versioning-release.md](./versioning-release.md)): additive fields do not bump it, a change a
  version-1 reader would misread does.
* A file version 1 gained later (§6: `private_hub_accesses`, `container_milestones`, `structure_presets`) is additive
  too: every archive written since carries it, and one written before reads as holding none of its
  rows.
* A section is a file one archive carries beyond §6's entities, named in its manifest like any
  member and read by no restore. Only the copy a person gets of their own data has one, `kudos`
  (§9).
* A field a writer stops writing is additive in the other direction: a reader tolerates it in an
  older archive and takes nothing from it. `accounts` no longer carries `ai_consent`, a column
  nothing reads or writes - the person's AI objection is a `consent_records` row
  ([data-protection.md](./data-protection.md) §4.1), and an always-false flag beside it read as a
  consent nobody gave. An import ignores it in an archive written before; the column leaves the
  schema in a later contract migration.
* Readers refuse a `format_version` above what they know, and accept everything from the minimum
  readable version (currently 1) up.
* This document is versioned with the repository; the manifest's `product_version` and
  `schema_version` say which state of it applied when an archive was written.

## 12. What exit law asks, and what exists

**Not legal advice:** the obligations are the reading the research of 2026-10 records
([foundation-law-standards-2026-10.md](../backlog/suite/research/foundation-law-standards-2026-10.md)
§1.2, §1.6, §1.8). Each binds the **operator** of a shared installation — a provider is the data
processing service of the Data Act, the ICT third-party provider of DORA, the processor of the
GDPR — and Hubtask supplies what the operator needs to meet it.

| Law | What it asks of the operator | What Hubtask supplies | Not Hubtask's |
|---|---|---|---|
| GDPR Art. 20 | A person's own data, machine-readable | The data subject export, JSON Lines with the schema ([data-protection.md](./data-protection.md) §4) | — |
| Data Act Art. 23–26 (since 2025-09-12) | All exportable data and digital assets of a customer, in a structured, commonly used, machine-readable format, with information on what is exported and how | This format: the whole workspace — its structure, entries, comments, files, people and groups, roles, custom field definitions, templates, views, rules and the trail — documented here and readable without Hubtask (§2), never encrypted (§1), credentials left out (§9) | The notice period (at most two months), the transition (30 days), the retrieval window (at least 30 days): contract terms |
| Data Act Art. 29 (zero charges from 2027-01-12) | Switching without charges | The export costs nothing in the software; nothing gates it behind a plan or an entitlement | The provider's prices |
| Data Act Art. 30 (SaaS) | Open interfaces, compatibility with open specifications | The public OpenAPI contract, this versioned format (§11) | — |
| DORA Art. 28(8), Art. 30 | An exit strategy; return of the data on termination | The same export; and, as the last exit, the same software self-hosted ([UC-BAK-07](../usecases/backup/UC-BAK-07-take-the-workspace-to-another-installation.md)) | The exit plan, the transition period |

**The import exists**, for a Hubtask installation: a restore of the archive as a new workspace
(§10, UC-BAK-07, `test/integration/restore_foreign_test.go`). The export is the control plane's act
in a shared installation; a workspace serves itself through its own backups to its own targets
(UC-BAK-01 … 03, the same format, backup-restore.md §9) where the operator allows workspace targets
(`HUBTASK_BACKUP_TENANT_TARGETS`, backup-restore.md §2), and in a single-workspace installation its
owner is the operator.

**The rule for what comes:** a new kind of workspace data — a type, a field, a link, a form, a
workflow, a page, a whiteboard, a record, a time entry, a goal — joins §6 in the step that builds
it, with its references and its order; configuration is data and is never on the exclusion list of
§9. The test of §9 already holds every table under row level security to one of the two.

**Not yet** — named gaps, each owned by a step of [the suite plan](../backlog/suite/README.md):

* The provenance of an AI-originated value does not travel: suggestions are AI machinery (§9) and
  the accepted value carries no mark ([ai-first.md](./ai-first.md) §2.1, AI-2).
* No import from other tools' exports (issue trackers, wikis) with their configuration, history and
  attachments.
* A workspace of a shared installation whose operator allows no workspace targets cannot take its
  data out without the operator; the Data Act asks for no obstacle. Closing it means a self-service
  export for the workspace's owner, which UC-BAK-07's *Where it ends* (no direct download, the
  operator's act) rules out today — an owner's decision.
