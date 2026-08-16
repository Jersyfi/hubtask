# Data Catalogue

A record of every data category the system processes, with its purpose, classification, storage
locations, retention, and deletion path. The basis for the record of processing activities
(GDPR Art. 30) that every operator keeps for themselves.

* **Version:** 0.1.0 · **As of:** 2026-08-14 · **Maintenance:** by pull request, so changes are traceable
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
| Invitations (email, creator, status) | `invitation` | `PERSONAL_BASIC` | Onboarding | Contract | 30 days after completion | `RETENTION` |

---

## 3. Business content

| Data category | Table / location | Classification | Purpose | Retention | Deletion path |
|---|---|---|---|---|---|
| Title, description, notes | `work_item` | `PERSONAL_CONTENT` | The core feature | Until deleted by the user | `CASCADE` (trash 30 days) |
| Due date, status, ordering, path | `work_item` | `NON_PERSONAL` | The core feature | As above | `CASCADE` |
| Assignments | `item_member` | `PERSONAL_BASIC` | Work organisation | As above | `CASCADE` / `ANONYMIZE` |
| Comments | `comment` | `PERSONAL_CONTENT` | Collaboration | As above | `CASCADE` / `ANONYMIZE` (the author) |
| Activity history | `activity_entry` | `PERSONAL_CONTENT` | Traceability within the product | With the item | `CASCADE` |
| Attachments (file, name, checksum) | `media_object`, object storage | `PERSONAL_CONTENT` | The core feature | As above | `CASCADE` + object deletion after reference counting |
| Custom fields (values) | `work_item.custom_fields` | `PERSONAL_CONTENT` | Extensibility | As above | `CASCADE` |
| Containers, buckets, labels | `container`, `bucket`, `label` | `NON_PERSONAL` | Structure | As above | `CASCADE` |
| Full-text index | `work_item.search_vector`, optionally a vector index | `PERSONAL_CONTENT` (derived) | Search | With the source row | `CASCADE` |
| Templates, saved views | `template`, `saved_view` | `PERSONAL_CONTENT` (free text possible) | Productivity | Until deleted | `CASCADE` |
| Jumble entries (raw text from mail/webhook) | `jumble_entry` | `PERSONAL_CONTENT` | Quick capture | Until converted, otherwise 90 days | `RETENTION` |
| Trash marker | `work_item.deleted_at` | `NON_PERSONAL` | Restore | 30 days | `RETENTION` (hard delete) |

---

## 4. Automation, integration, AI

| Data category | Table / location | Classification | Purpose | Retention | Deletion path |
|---|---|---|---|---|---|
| Automation rules (conditions, actions) | `automation_rule` | `NON_PERSONAL` (references to people possible) | Automation | Until deleted | `CASCADE` |
| Rule runs (input as a reference, result) | `rule_run` | `PERSONAL_TECHNICAL` | Debugging, transparency | 30 days | `RETENTION` |
| Webhook subscriptions (target URL, secret) | `webhook_subscription` | `SECRET` (the secret) + `NON_PERSONAL` | Integration | Until deleted | `CASCADE` |
| Delivery logs (status, truncated body) | `webhook_delivery` | `PERSONAL_TECHNICAL` | Proof of delivery | 30 days | `RETENTION` |
| Domain events (references, metadata) | `outbox_event` | `PERSONAL_TECHNICAL` | Integration | 7 days after delivery | `RETENTION` |
| Calendar feeds (token, scope) | `calendar_feed` | `SECRET` + `PERSONAL_CONTENT` (titles in the ICS) | Calendar integration | Until revoked | `CASCADE` |
| AI usage metadata (provider, model, purpose, scope, timestamp) | `audit_log` | `PERSONAL_TECHNICAL` | Transparency, evidence of third-country transfer | The audit period | `IMMUTABLE` |
| Content transmitted to AI | **not stored** (transmitted to the provider) | `PERSONAL_CONTENT` | AI suggestions | Not stored in the system; at the provider per their agreement | — |
| Embedding vectors (optional) | `item_embedding` | `PERSONAL_CONTENT` (derived) | Semantic search | With the source row | `CASCADE` |
| Email intake (sender, subject, body) | `jumble_entry` | `PERSONAL_CONTENT` | Quick capture | See §3 | `RETENTION` |

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
