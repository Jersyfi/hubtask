# Audit Concept

Binding for all building blocks. Decision: [ADR-0017](../adr/ADR-0017-audit-trail.md). Related:
[security.md](./security.md) (T-14, T-15), [data-protection.md](./data-protection.md).

---

## 1. Four separate kinds of record

Hubtask never mixes operations, product history and evidence in one log:

| Kind | Storage | Purpose | Audience | Mutable? | Retention |
|---|---|---|---|---|---|
| **Audit trail** | Table `audit_log` | Evidence of security- and privacy-relevant events | Auditor, data protection officer, tenant administrator | **No** (append-only, enforced by the database) | 400 days by default; configurable |
| **Business history** | Table `activity_entry` | "Who moved this task?" — part of the product | End users | No (but deleted with the item) | The lifetime of the item |
| **Operational logs** | stdout/aggregator | Debugging | Operators | Ephemeral | 7–30 days |
| **Domain events** | `outbox_event` | Integration, automation | Systems | No | 7 days after delivery |

The audit trail is **not** a by-product of the events: it also records what changes nothing — a
failed login, a downloaded export, a refused permission check.

**The 400-day default** is a year plus a quarter, so an annual review still reaches the start of the
year it reviews. It is a default, not a ceiling: a tenant under its own obligation configures
another. An audit entry is personal data under a legitimate interest, which is why it is not longer.

---

## 2. The structure of an audit entry

| Field | Contents |
|---|---|
| `id` | UUIDv7 (monotonic, time-sorted) |
| `tenant_id` | The tenant, always. An act about the installation rather than a tenant is recorded in `instance_event` (§6), not here |
| `occurred_at` | `timestamptz` (UTC), server time |
| `action` | A stable code, e.g. `item.deleted`, `member.role_changed`, `auth.login_failed`, `export.downloaded` |
| `outcome` | `SUCCESS` \| `DENIED` \| `FAILED` |
| `severity` | `INFO` \| `NOTICE` \| `WARNING` \| `CRITICAL` |
| `actor_type` | `USER` \| `SERVICE_ACCOUNT` \| `AUTOMATION` \| `AI_AGENT` \| `SYSTEM` |
| `actor_id`, `actor_label` | The identifier and the label valid at the time (the name stays readable after the account is deleted) |
| `on_behalf_of_id` | For automation and agents: the principal from `run_as` |
| `target_type`, `target_id`, `target_label` | The affected object |
| `context` | `request_id`, `trace_id`, `ip_truncated`, `user_agent_class`, `api_client`, `rule_id` |
| `changes` | A structured diff of **only the changed fields**, each value masked by its classification (§4) |
| `legal_basis` | For privacy-relevant events: the legal basis or occasion (e.g. `dsr.erasure`) |
| `prev_hash`, `hash` | The hash chain (§3) |
| `seq` | A tenant-local, gapless sequence number |

Actor and target labels are stored **denormalised**: an entry that only points at a foreign key
becomes unreadable once the account is deleted.

---

## 3. Immutability

Three levels:

1. **No rights:** `hubtask_app` holds only `INSERT` and `SELECT` on `audit_log` — no `UPDATE`,
   `DELETE` or `TRUNCATE`. Enforced by `GRANT`, checked by gate SG-4.
2. **Trigger lock:** a `BEFORE UPDATE OR DELETE` trigger raises an exception. One narrow exception: a
   `DELETE` passes while a transaction-scoped marker names exactly the row's tenant — set only by
   `purge_tenant_trail`, the hard delete's act, which clears it before it returns (§6).
3. **Hash chain:** `hash = SHA-256(prev_hash ‖ canonical serialisation of the entry)`, one chain per
   tenant, plus a gapless `seq`. `POST /audit:verify` walks the chain and reports the break point
   and any sequence gap.

**The hash covers the entry as the row gives it back**, not as the caller built it. Four rules
follow, all in the adapter (the port and the domain know nothing about PostgreSQL), each held by a
test:

* **The tail is the highest `seq`, never the newest timestamp.** Callers read the clock before they
  queue for the chain's per-tenant lock, so timestamps and sequence numbers disagree under
  concurrency; the unique index `(tenant_id, occurred_at, seq)` cannot catch a repeated `seq`.
  Verification walks by `seq` too.
* **The instant is truncated to microseconds**, what `timestamptz` keeps.
* **The changed fields go through the reader's encoder before hashing**, because `JSONB` returns keys
  in its own order.
* **An entry that changed nothing hashes `{}`, not `null`.**

**A deliberate limit:** the chain proves tampering **inside** the database, not against somebody with
full database access who recomputes it. For that, the chain end is anchored outside:

* **Configuration:** `PUT /audit/anchoring` (`ConfigureAuditAnchoring`) names one of the workspace's
  own backup targets, or `null` to switch anchoring off. It needs `STRUCTURE` and is audited
  (`audit.anchoring_configured`). The setting is `audit_anchor_target_id` in the workspace settings,
  read back on `GET /tenant`. It is the workspace administrator's (the audit screen, `hubctl audit
  anchor [--target <id> | --off]`); the operator has no say, because nothing here crosses a
  workspace.
* **The job** `audit.anchor` is per tenant and self-seeded by the configuration's write
  ([multi-tenancy.md](./multi-tenancy.md) §2.1); it reschedules itself to shortly after the next
  midnight UTC and finishes when no target is named. Once a day, where the chain moved, it writes
  `hubtask-anchor-<tenant>-<YYYYMMDD>.json` (sequence, hash, moment, product version) to the target
  and records an `audit_anchor` row with the target as `destination` and the object's SHA-256 as
  `receipt`. The row is append-only for the application role.
* **The check:** `POST /audit:verify` with `anchors: true` also reads the last anchor back, checks the
  receipt, and compares the file's hash with the chain end this database **derives** at that
  sequence (each link recomputed from the previous derived hash). A chain rewritten below the anchor
  is reported at the anchor and at the break; one rewritten and recomputed whole is reported at the
  anchor alone. The answer carries `anchoring_configured`, `anchored_until`, `anchor_seq`,
  `anchor_agrees` and `anchor_error_code` (`audit.anchor_unreadable`,
  `audit.anchor_receipt_mismatch`); a workspace without a target is told so.
* **The target's protection:** object lock and a credential that cannot delete or shorten the
  retention ([backup-restore.md](./backup-restore.md) §12). The lock retention should be **no
  shorter than the audit period** (400 days by default) — an anchor that expires before the entries
  it seals cannot be checked. 400 files of a few hundred bytes are under a megabyte.

Entries are deleted only by the retention job, a month partition at a time, never individually. The
deletion writes an entry with the count and the period.

---

## 4. What gets audited — and what does not belong in it

The full list is [docs/audit/event-matrix.md](../audit/event-matrix.md), generated from the use case
catalogue by `make generate`, so it describes what this build records. By area:

| Area | Events |
|---|---|
| Authentication | Login (success/failure/lockout), logout, MFA enabled/disabled, password change, refresh token reuse detected, step-up, an account connected to a sign-in provider (`LINK`) |
| Tokens | PAT created/revoked/first used, service account created, OAuth2 consent granted/withdrawn |
| Permissions | Role changed, invitation created/accepted/revoked, group changed, **denied access** (`outcome=DENIED`). Planned, not built (milestone PH): a private hub opened through the emergency access, a managed account's start password issued |
| Tenant | Provisioning, suspension, settings change, deletion requested/executed |
| Data | Deletion, restore, trash emptied, archiving, bulk operation (with its scope), import |
| Export/access | Export requested, produced, **downloaded**, link expired; access to media marked sensitive |
| Automation | Rule created/changed/enabled/disabled, rule auto-disabled for a loop or errors, outbound call blocked (SSRF) |
| AI | Agent action executed, AI processing with a third party, AI feature enabled/disabled |
| Integration | Webhook subscription created/changed, calendar feed created/revoked |
| Data protection | Data subject request created/fulfilled/rejected (extended: planned, milestone PH), retention period changed, anonymisation performed, data breach documented |
| Administration | Security-relevant configuration change, sign-in provider configured/offered/removed, migration executed, audit retention changed |

**Never** in the audit trail: the content of tasks, notes, comments or attachments; passwords,
tokens or secrets in any form; full IP addresses (truncated: IPv4 /24, IPv6 /48); AI prompts and
responses in clear text (metadata only: provider, model, purpose, scope).

**Masking of `changes`.** Three masking levels, derived from the six data classes of
[data-protection.md](./data-protection.md) §3 by `audit.MaskingFor`:

| Masking | Written as | Classes |
|---|---|---|
| `OPEN` | The value in clear (e.g. `OPEN → DONE`) | `NON_PERSONAL`, `PERSONAL_TECHNICAL` |
| `SENSITIVE` | "changed" plus a hash for comparability | `PERSONAL_BASIC`, `PERSONAL_CONTENT`, `SPECIAL_CATEGORY_RISK` |
| `SECRET` | Not at all | `SECRET` |

A field with no classification is masked, never opened: gate PG-1 refuses one at build time and the
masking refuses one at run time. A trail that kept every title in full would undermine the deletion
obligation of the item it documents.

---

## 5. Access and analysis

| Role | Visibility |
|---|---|
| Tenant `OWNER`/`ADMIN` | The full audit trail of their own tenant |
| `MEMBER` | Their own events (`actor_id = self`) |
| Instance administrator (self-hosted/provider) | System-wide events; **no** blanket insight into tenant trails without a documented occasion, which is itself audited |
| Auditor | The `AUDITOR` role: the audit trail and the configuration, **no** content |

**The `AUDITOR` role** is not a rung on the ladder of the other six: it carries only `AUDIT_READ` and
`READ_CONFIGURATION`, so somebody who needs more holds two memberships and the rights add up
(`core/domain/service.Allows`). `READ_CONFIGURATION` is split out of `STRUCTURE` (a writing
permission; [domain-model.md](./domain-model.md) §3.2) and reaches the backup targets and runs, the
retention rules and their previews, the legal holds, the automation rules and their runs, the
webhook subscriptions, and the AI provider configuration. It reaches no secret (a signing secret is
answered once at creation; target credentials are sealed) and no content. Without the
configuration an auditor could not judge an entry like "a retention rule removed 400 objects"
without asking the audited party.

**A renamed action stays one action.** An action code may be renamed so a family reads as one, but a
stored entry is never rewritten — the hash covers the stored shape (§3). New entries carry the new
name, old entries keep theirs, and the `action` filter matches both names and the family. The renames
are listed in `audit.Renamed` (`core/port/audit/Renamed.go`).

**Routes:** `GET /audit` with the shared query DSL (period, `action`, `actor`, `target`, `outcome`);
`POST /audit:export` as a signed JSON Lines or CSV archive with a checksum manifest and a stated
period; `POST /audit:verify` for the chain check. Scopes: `audit:read` for reading and verifying,
`audit:export` for the export — carrying a copy out is a different act.

Every audit export produces an audit entry. A **read** does not (the trail would grow by being
read); a refused read does, and so does a verification that finds the chain broken
(`audit.chain_broken`, critical).

**The export is written in the clear**, and the manifest says so: it is read outside this
installation. "Signed" means the manifest's digest sealed under the installation's master key and
bound to that export — proof the archive was produced here and not altered, to anybody who can ask
this installation. An installation without a key writes no signature and records that.

---

## 6. Auditability of the system (not just of its users)

Machine-readable evidence a reviewer can check without taking the operator's word:

| Evidence | Artefact |
|---|---|
| What is running here? | `product_version` in `GET /meta/capabilities` and the `hubtask_build_info` metric; a signed image with provenance |
| What is it made of? | An SBOM (CycloneDX) per release |
| Which controls are in force? | The pipeline's gate report per release (SG-1…SG-13, RT-1…RT-12), archived as an artefact |
| Which data is processed? | [data-catalog.md](../privacy/data-catalog.md), versioned in the repository |
| Which decisions were taken? | The ADRs, with date and status |
| Has restore been verified? | The restore drill record per release (RT-9) |
| Who has access? | An access review — memberships, roles, tokens, service accounts, last use. Planned: no route serves it yet |

This replaces no certification (ISO 27001 and SOC 2 are organisational); it supplies the technical
evidence.

### The audit's own life: pseudonymisation instead of deletion

**An erasure request does not reach the trail.** Deleting the entries about a person would delete
the record that their request was handled. The trail is kept under the evidentiary interest for its
retention period (§1); the entries are a record of what was done — no content, a truncated address,
a user agent class, no free text.

**It cannot be edited in place.** No `UPDATE` grant, a trigger refusing one, and every field an
erasure would change is covered by the hash chain (§3).

So pseudonymisation happens at the two points where it can:

* **At the boundary.** Once an account is erased, a read and an export of the trail answer the actor
  as a pseudonym derived per tenant from the identifier (`audit_pseudonym`), not as the stored label.
  The row is untouched, the chain still verifies, and one actor's entries stay one actor's.
* **At the end of life.** When the retention period is up, the partition is dropped whole (§3) — the
  same deletion for everybody.

The erasure itself is audited with `legal_basis = dsr.erasure`: the entry about the erasure is the
one that has to survive it.

### The instance's own journal: evidence where a per-tenant trail cannot live

The hard delete of a tenant ([multi-tenancy.md](./multi-tenancy.md) §5) removes the tenant's trail
by design. Its evidence, and that of every control-plane act, lives in **`instance_event`**: one row
per act — `tenant.provisioned`, `tenant.suspended`, `tenant.resumed`, `tenant.deletion_requested`,
`tenant.hard_deleted`, `tenant.password_opened`, `tenant.password_closed`,
`instance.session_elevated`. A row carries identifiers, the slug, the acting operator's label,
moments and counts (for a hard delete: rows, media objects and bytes, outbox events, queued jobs and
trail entries removed; for an opening of the password: its end, and that a requester and a reason
were given — never their texts, which stay in the workspace's own trail). Never content.

* **It belongs to no tenant.** No row policy ([multi-tenancy.md](./multi-tenancy.md) §2.1). It is
  written only by the tenant lifecycle and instance use cases, append-only for the application role,
  and served by no API — the operator reads it at the database.
* **It commits with the act.** The hard delete writes the evidence, purges the trail through
  `purge_tenant_trail`, and lets the cascade take the rest, in one transaction.
* **It is not chained.** The chain exists so tenants can prove their trail to an outside party; here
  it would attest the operator to the operator. Its integrity rests on the grants.

Lifecycle acts (provisioning, suspension) are also written to the tenant's own trail where one
exists. The journal is the floor under the trail, not a copy of it.

---

## 7. Architectural integration

* Auditing is **not an adapter concern.** The entry is written in the application layer, in the same
  transaction as the business change: no event without an entry, no entry without an event. MCP and
  automation therefore audit exactly as REST does.
* The use case registry carries an `audit` declaration per use case (mandatory/optional, the action
  code, the target type, the classification). A use case with security or privacy relevance and no
  declaration fails the build (gate SG-13).
* Denied access is recorded in the `AuthorizationService`, the one place authorisation happens, so
  `outcome=DENIED` is complete without anybody remembering it.
* Writes stay cheap: one insert, no foreign key checks on actor or target, monthly partitions, and
  per-tenant indexes on time, `seq`, action, actor and target only.

---

## 8. Gates and tests

| ID | Check |
|---|---|
| SG-13 | Every use case with security or privacy relevance has an audit declaration (reconciled against the registry) |
| AT-1 | `UPDATE`/`DELETE` on `audit_log` fails under the app role (both the grant **and** the trigger) |
| AT-2 | The hash chain and `seq` are gapless after 1,000 mixed events; a tampered row is found by `:verify` |
| AT-3 | Denied access produces `outcome=DENIED` with the correct actor |
| AT-4 | `changes` contains no `SENSITIVE` value in clear text and no secret |
| AT-5 | The audit entry and the business change are atomic (a rollback leaves no entry) |
| AT-6 | The automation and MCP paths produce the same audit entries as the REST path (channel parity) |
| AT-7 | Deleting an account leaves its audit entries readable (the denormalised `actor_label`) |

An older prefix, `AU-n`, still appears in places: AU-1 is SG-13, AU-2 is AT-1, AU-4 is AT-4, and the
set AU-1…AU-7 is AT-1…AT-7.

---

## 9. Open points

| # | Point | Needed by |
|---|---|---|
| A-3 | Establish the need for a SIEM connection (syslog/CEF export or pull through the API) | After `1.0.0` |

Closed points cited elsewhere: A-1 (the 400-day default) is §1; A-2 (external anchoring) is §3; A-4
(the auditor reads the configuration) is §5.
