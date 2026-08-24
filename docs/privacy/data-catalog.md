# Data Catalogue

A record of every data category the system processes, with its purpose, classification, storage
locations, retention, and deletion path. The basis for the record of processing activities
(GDPR Art. 30) that every operator keeps for themselves.

* **Version:** 0.3.0 · **As of:** 2026-08-22 · **Maintenance:** by pull request, so changes are traceable
* **Concept:** [../architecture/data-protection.md](../architecture/data-protection.md)
* **Consistency check:** gate PG-7 compares this record against the database schema; a table with personal content that is missing here fails the build.

> This document describes the software. It is **not** an operator's record of processing activities
> and **not** legal advice — legal bases and recipients depend on the specific installation. The
> "typical legal basis" column is a prompt for the conversation with a data protection officer, not
> an assessment.

---

## 1. Legend

**Classification:** `NON_PERSONAL` · `PERSONAL_BASIC` · `PERSONAL_CONTENT` ·
`PERSONAL_TECHNICAL` · `SECRET` (see the concept, §3)

**Deletion path:** `CASCADE` (with the parent object) · `ANONYMIZE` (the reference is removed, the
record remains) · `RETENTION` (a period job) · `IMMUTABLE` (only through audit retention) ·
`HASH_ONLY` (never stored in clear text)

---

## 2. Identity and access data

| Data category | Table / location | Classification | Purpose | Typical legal basis | Retention | Deletion path |
|---|---|---|---|---|---|---|
| Display name, avatar reference | `account` | `PERSONAL_BASIC` | Presentation, attribution of tasks | Contract / legitimate interest | The lifetime of the account | `ANONYMIZE` or `CASCADE` |
| Email address | `account` | `PERSONAL_BASIC` | Sign-in, invitation, notification | Contract | The lifetime of the account | `CASCADE` |
| Password hash | `account` | `SECRET` | Authentication | Contract | The lifetime of the account | `HASH_ONLY`, `CASCADE` |
| Locale, time zone, start of week | `account` | `PERSONAL_BASIC` | Localisation | Contract | The lifetime of the account | `CASCADE` |
| MFA secret, recovery codes | `account_mfa` | `SECRET` | Two-factor authentication | Contract / legal obligation | The lifetime of the account | `CASCADE` |
| External identity (OIDC `sub`, issuer) | `account_identity` | `PERSONAL_BASIC` | Single sign-on | Contract | The lifetime of the link | `CASCADE` |
| Membership, role, groups | `membership`, `account_group_member` | `PERSONAL_BASIC` | Permissions | Contract | The lifetime of the membership | `CASCADE` |
| Token hash, scopes, label | `access_token` | `SECRET` (+ `PERSONAL_BASIC` for the name) | API access | Contract | Until revocation/expiry, max. 1 year | `CASCADE` |
| Last token use (time, truncated IP) | `access_token` | `PERSONAL_TECHNICAL` | Abuse detection | Legitimate interest | 90 days | `RETENTION` |
| Sessions (refresh family, device characteristics, truncated IP) | `session` | `PERSONAL_TECHNICAL` | Session management, security | Legitimate interest | 30 days after expiry | `RETENTION` |
| Failed sign-ins, lockout counters | `login_attempt` | `PERSONAL_TECHNICAL` | Brute force protection | Legitimate interest | 30 days | `RETENTION` |
| Invitations (address, status) | `account` (status `INVITED`) | `PERSONAL_BASIC` | Onboarding | Contract | With the account; an invitation never accepted is an account never used | `CASCADE` |
| Who invited whom, and when | `audit_log` | `PERSONAL_TECHNICAL` | Evidence of how somebody got access | Legitimate interest | The audit period | `IMMUTABLE` |
| Queued invitation message (identifiers only) | `job` | `PERSONAL_TECHNICAL` (references) | Delivering the invitation | Contract | 7 days after completion | `RETENTION` |
| Workspace (name, slug, locale, settings) | `tenant` | `NON_PERSONAL` | The boundary everything else sits in | Contract | The lifetime of the workspace | `CASCADE` (everything below it) |
| Group (name, description) | `account_group` | `NON_PERSONAL` | Permissions in bulk | Contract | The lifetime of the group | `CASCADE` |
| Consent (purpose, granted, revoked, source) | `consent_record` | `PERSONAL_BASIC` | Evidence of a consent given or withdrawn | Consent (Art. 7(1) as evidence) | 3 years after withdrawal | `RETENTION` |
| Device (platform, name, last seen, push token, cursor) | `sync_device` | `PERSONAL_TECHNICAL` | Offline synchronisation, push | Contract | 90 days after the last contact | `RETENTION` |

---

## 3. Business content

| Data category | Table / location | Classification | Purpose | Retention | Deletion path |
|---|---|---|---|---|---|
| Title, description, notes | `work_item` | `PERSONAL_CONTENT` | The core feature | Until deleted by the user | `CASCADE` (trash 30 days) |
| Due date, status, ordering, path | `work_item` | `NON_PERSONAL` | The core feature | As above | `CASCADE` |
| Assignments | `item_member`, `work_item.assignee_id` | `PERSONAL_BASIC` | Work organisation | As above | `CASCADE` (the member link, with the entry) / `ANONYMIZE` (the assignee: the account's deletion clears the reference and leaves the entry) |
| Comments | `comment` | `PERSONAL_CONTENT` | Collaboration | As above | `CASCADE` / `ANONYMIZE` (the author) |
| Activity history | `activity_entry` | `PERSONAL_CONTENT` | Traceability within the product | With the item | `CASCADE` |
| Attachments (file, name, checksum) | `media_object`, object storage | `PERSONAL_CONTENT` | The core feature | As above | Reference counting plus the reconciliation job: an object that nothing points at any more is marked, its bytes are removed from storage, and the row goes with a deletion journal entry and a tombstone (C-06, `data-protection.md` §5). The file name is the only free text on the row and travels with it |
| Custom fields (values) | `work_item.custom_fields`, with `work_item.custom_field_refs` naming the definition each value was written under (`NON_PERSONAL` — definition identifiers) | `PERSONAL_CONTENT` | Extensibility | As above | `CASCADE`; a value whose definition was deleted is hidden from every read and goes with the entry (C-07) |
| Containers, buckets, labels | `container`, `bucket`, `label` | `NON_PERSONAL` | Structure | As above | `CASCADE` |
| Full-text index | `work_item.search_document` (and `work_item.search_vector` until a later migration drops it), optionally a vector index | `PERSONAL_CONTENT` (derived) | Search | With the source row | `CASCADE` |
| Content language | `work_item.content_language` | `NON_PERSONAL` | Which text search configuration the entry is indexed under (C-08, ADR-0034) | With the source row | `CASCADE` |
| Templates, saved views | `template`, `saved_view` | `PERSONAL_CONTENT` (free text possible) | Productivity | Until deleted | `CASCADE` |
| Jumble entries (raw text from mail/webhook) | `jumble_entry` | `PERSONAL_CONTENT` | Quick capture | Until converted, otherwise 90 days | `RETENTION` |
| Trash marker | `work_item.deleted_at` | `NON_PERSONAL` | Restore | 30 days | `RETENTION` (hard delete) |
| Label and attachment links | `item_label`, `item_attachment` | `NON_PERSONAL` | Structure; the content sits in the row they point at | With the item | `CASCADE` |
| Set membership for offline merging (labels, members, watchers, attachments with their tags) | `set_element` | `PERSONAL_BASIC` (member references) | Merging two devices' edits without losing either | With the item | `CASCADE` |
| Custom field definitions (key, type, options) | `custom_field_definition` | `NON_PERSONAL` | Extensibility; the values are in `work_item` | Until deleted | `CASCADE` |
| Capability profiles | `item_capability_profile` | `NON_PERSONAL` | What an item type can do | Permanent (system rows) or until deleted | `CASCADE` |
| Reminders (time, channels, recipients) | `reminder` | `PERSONAL_BASIC` (the recipients) | Punctuality | With the item | `CASCADE` |
| Recurrence rules (RRULE, time zone, horizon) | `recurrence_rule` | `NON_PERSONAL` | Recurring work | With the item | `CASCADE` |
| Assignment policies (strategy, candidates, round-robin state) | `auto_assign_policy` | `PERSONAL_BASIC` (the candidates) | Distributing work | Until deleted | `CASCADE` |

---

## 4. Automation, integration, AI

| Data category | Table / location | Classification | Purpose | Retention | Deletion path |
|---|---|---|---|---|---|
| Automation rules (conditions, actions) | `automation_rule` | `NON_PERSONAL` (references to people possible) | Automation | Until deleted | `CASCADE` |
| Rule runs (input as a reference, result) | `rule_run` | `PERSONAL_TECHNICAL` | Debugging, transparency | 30 days | `RETENTION` |
| Webhook subscriptions (target URL, secret) | `webhook_subscription` | `SECRET` (the secret) + `NON_PERSONAL` | Integration | Until deleted | `CASCADE` |
| Delivery logs (status, truncated body) | `webhook_delivery` | `PERSONAL_TECHNICAL` | Proof of delivery | 30 days | `RETENTION` |
| Domain events (references, metadata) | `outbox_event` | `PERSONAL_TECHNICAL` | Integration | 7 days after delivery | `RETENTION` |
| Change log (state deltas incl. field snapshots) | `change_log` | `PERSONAL_CONTENT` (the payload mirrors the row) | Offline synchronisation | The maximum offline window, 90 days by default (offline-sync.md §7) | `RETENTION` (a purge before it elapses would let a device recreate deleted objects) |
| Calendar feeds (token, scope) | `calendar_feed` | `SECRET` + `PERSONAL_CONTENT` (titles in the ICS) | Calendar integration | Until revoked | `CASCADE` |
| AI usage metadata (provider, model, purpose, scope, timestamp) | `audit_log` | `PERSONAL_TECHNICAL` | Transparency, evidence of third-country transfer | The audit period | `IMMUTABLE` |
| Content transmitted to AI | **not stored** (transmitted to the provider) | `PERSONAL_CONTENT` | AI suggestions | Not stored in the system; at the provider per their agreement | — |
| Embedding vectors (optional) | `item_embedding` | `PERSONAL_CONTENT` (derived) | Semantic search | With the source row | `CASCADE` |
| Email intake (sender, subject, body) | `jumble_entry` | `PERSONAL_CONTENT` | Quick capture | See §3 | `RETENTION` |
| What a subscriber has already consumed (consumer, event, time) | `event_consumption` | `NON_PERSONAL` | At-least-once delivery without a repeated effect (ADR-0007) | With the event, 7 days after delivery | `RETENTION` |
| Device operation log (operation, result, response) | `sync_op_log` | `PERSONAL_CONTENT` (the response mirrors the row) | Idempotent `:push` for offline devices | The offline window, 90 days by default | `RETENTION` |
| Tombstones (entity, identifier, deletion time) | `tombstone` | `PERSONAL_TECHNICAL` (references only) | So a device that was offline learns of a deletion instead of recreating it | Until the purge date, the offline window | `RETENTION` |

---

## 5. Operations, audit, compliance

| Data category | Location | Classification | Purpose | Retention | Deletion path |
|---|---|---|---|---|---|
| Audit trail (metadata, masked diffs, truncated IP) | `audit_log` | `PERSONAL_TECHNICAL` | Evidence, security | Configurable, 400 days by default | `IMMUTABLE` (retention only) |
| Data subject requests (kind, deadline, assignee, reason) | `data_subject_request` | `PERSONAL_BASIC` | Fulfilling data subject rights | 3 years (evidentiary interest, P-1 to be settled) | `RETENTION` |
| Retention rules | `retention_policy` | `NON_PERSONAL` | Storage limitation | Permanent | — |
| Jobs (type, parameters as references) | `job` | `PERSONAL_TECHNICAL` | Background processing | 7 days after completion | `RETENTION` |
| Idempotency keys | `idempotency_key` | `PERSONAL_TECHNICAL` | Protection against double processing | 24 hours | `RETENTION` |
| Usage figures (aggregates) | `usage_record` | `NON_PERSONAL` | Billing (opt-in) | 3 years when enabled | `RETENTION` |
| Export archives | Object storage | `PERSONAL_CONTENT` | Access, portability | 7 days, then automatic deletion | `RETENTION` |
| Operational logs | stdout / the operator's aggregator | `PERSONAL_TECHNICAL` | Debugging | 7–30 days (the operator) | The operator |
| Metrics | Prometheus | `NON_PERSONAL` | Operations | The operator | The operator |
| Traces | The OTel backend | `PERSONAL_TECHNICAL` (masked) | Debugging | 7 days recommended | The operator |
| Backups | The operator's infrastructure | Every class | Recoverability | A documented period (P-5) | Expiry of the cycle |
| Backup targets (kind, configuration, encrypted credentials) | `backup_target` | `SECRET` (the credentials) + `NON_PERSONAL` | Where a backup goes (ADR-0019) | Until deleted | `CASCADE` |
| Backup and restore runs (status, manifest, sizes, who asked) | `backup_schedule`, `backup_run`, `restore_run` | `PERSONAL_TECHNICAL` (the actor references) | Evidence that a backup happened and a restore was approved | With the archive's retention | `RETENTION` |
| Retention runs (what matched, what was blocked) | `retention_run` | `PERSONAL_TECHNICAL` (counts and reasons, no content) | Evidence of storage limitation | 400 days, like the audit period | `RETENTION` |
| Deletion evidence (entity, identifier, reason, time) | `deletion_journal` | `PERSONAL_TECHNICAL` (references only) | Proof that an erasure was carried out (Art. 17) | 3 years | `RETENTION` |
| Legal holds (scope, reason, who placed and released it) | `legal_hold` | `PERSONAL_TECHNICAL` | Suspending deletion for a legal reason | Until released, plus the evidentiary period | `RETENTION` |
| Audit anchors (chain hash, destination, receipt) | `audit_anchor` | `NON_PERSONAL` | Proof that the audit chain was not rewritten (ADR-0017) | With the audit period | `IMMUTABLE` |
| Privacy incidents (categories, count, description, measures) | `privacy_incident` | `PERSONAL_CONTENT` (the description is free text) | Art. 33/34 documentation | 3 years | `RETENTION` |

---

## 6. Recipients and sub-processors

There are **no** recipients prescribed by the project. Every outbound connection is optional,
individually switchable, and enumerated here in full:

| Recipient | When | Data transmitted | Default |
|---|---|---|---|
| The operator's SMTP relay | Notifications, invitations | Email address, task title, link | Configured by the operator |
| Object storage (S3/MinIO) | Attachments, exports | File content | A local volume as the alternative |
| OIDC provider | Sign-in | Identity attributes | **off** |
| AI provider | AI features | Free text content of the operation in question | **off**; a local model is recommended; a third country requires confirmation |
| External search index | Search | Indexed content | **off** |
| Webhook targets | Automation, integration | Event metadata and references | Configured by the tenant, with an allowlist in provider operation |
| CalDAV/ICS subscribers | Calendar integration | Titles, dates | Created by the user, revocable |
| **The vendor / the project** | — | **none** | No telemetry exists |

---

## 7. Maintaining this document

1. A new table or column with personal content → an entry here **in the same pull request** (the Definition of Done).
2. A new outbound connection → an entry in §6 and verification through gate PG-6.
3. A change of period → an entry here and in the `retention_policy` defaults.
4. Gate PG-7 compares the schema and the catalogue automatically; PG-2 verifies the deletion paths in practice.
5. An invitation is an account, not a separate record. There is no `invitation` table: the account
   exists in `INVITED` status from the moment somebody is invited, so a permission can be granted
   to it before the person signs in, and nothing they were given works until they do. A row with a
   redemption token arrives with the sign-in flow (`0.6.0`), because a token nobody can redeem is a
   credential lying around for months.
6. A partition is not a data category. `audit_log_2026_08` and the other partitions of `audit_log`
   and `change_log` are recorded through their parent table, and a gate reconciling the schema
   against this document has to resolve them to it rather than demand a row of their own.
