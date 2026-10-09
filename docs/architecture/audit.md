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
year it reviews. A tenant under its own obligation configures another; an audit entry is personal
data under a legitimate interest, which is why the default is not longer.

---

## 2. The structure of an audit entry

The columns are `audit_log` in [`db/schema.sql`](../../db/schema.sql). What they must hold:

* **`tenant_id`, always.** An act about the installation rather than a tenant goes to
  `instance_event` (§6), not here.
* **`id`** is a UUIDv7; **`occurred_at`** is server time in UTC; **`seq`** is tenant-local and
  gapless; **`prev_hash`, `hash`** form the chain (§3).
* **`action`** is a stable code (`item.deleted`, `auth.login_failed`); **`outcome`** is `SUCCESS`,
  `DENIED` or `FAILED`; **`severity`** `INFO` to `CRITICAL`.
* **`actor_type`** (`USER`, `SERVICE_ACCOUNT`, `AUTOMATION`, `AI_AGENT`, `SYSTEM`), `actor_id`, and
  for automation and agents `on_behalf_of_id`, the principal from `run_as`.
* **`context`** holds only references and reduced technical data: `request_id`, `trace_id`,
  `ip_truncated`, `user_agent_class`, `api_client`, `rule_id`.
* **`changes`** is a diff of **only the changed fields**, each value masked by its classification
  (§4); **`legal_basis`** names the occasion of a privacy-relevant event (e.g. `dsr.erasure`).
* **Actor and target labels are stored denormalised**, as valid at the time: an entry that only
  points at a foreign key becomes unreadable once the account is deleted.

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

**The hash covers the entry as the row gives it back**, not as the caller built it. Four rules in
the adapter, each held by a test: the tail is the **highest `seq`**, never the newest timestamp
(the two disagree under concurrency), and verification walks by `seq`; the instant is **truncated to
microseconds**; the changed fields go **through the reader's encoder** before hashing (`JSONB`
reorders keys); an entry that changed nothing hashes **`{}`, not `null`**.

**A deliberate limit:** the chain proves tampering **inside** the database, not against somebody with
full database access who recomputes it. For that, the chain end is anchored outside:

* **Configuration:** `PUT /audit/anchoring` names one of the workspace's own backup targets, or
  `null` to switch it off; it needs `STRUCTURE` and is audited (`audit.anchoring_configured`). It is
  the workspace administrator's setting; the operator has no say, because nothing here crosses a
  workspace.
* **The job** `audit.anchor` is per tenant, self-seeded by the configuration's write
  ([multi-tenancy.md](./multi-tenancy.md) §2.1), runs daily shortly after midnight UTC and finishes
  when no target is named. Where the chain moved, it writes `hubtask-anchor-<tenant>-<YYYYMMDD>.json`
  (sequence, hash, moment, product version) to the target and records an `audit_anchor` row
  (append-only for the application role) with the object's SHA-256 as `receipt`.
* **The check:** `POST /audit:verify` with `anchors: true` reads the last anchor back, checks the
  receipt, and compares the file's hash with the chain end this database **derives** at that
  sequence. A chain rewritten below the anchor is reported at the anchor and at the break; one
  rewritten and recomputed whole, at the anchor alone.
* **The target's protection:** object lock and a credential that cannot delete or shorten the
  retention ([backup-restore.md](./backup-restore.md) §12), with a lock retention **no shorter than
  the audit period** — an anchor that expires before the entries it seals cannot be checked.

Entries are deleted only by retention, a month partition at a time, never individually, and the
deletion writes an entry with the count and the period. **Not built yet:** nothing drops an
`audit_log` partition today, so the trail is kept beyond its period
([data-retention.md](./data-retention.md) §3, kind `AUDIT`).

---

## 4. What gets audited — and what does not belong in it

What this build records, use case by use case, is the generated
[event matrix](../audit/event-matrix.md) (`make generate`). In short: authentication, tokens,
permissions (including **denied access**, `outcome=DENIED`), the tenant lifecycle, deletion,
restore, import and bulk operations, exports (requested, produced, **downloaded**), automation, AI
use, integrations, data protection cases and security-relevant administration. Planned, not built
(milestone PH): a private hub opened through the emergency access, a managed account's start
password issued, a data subject request's deadline extended.

**Never** in the audit trail: the content of tasks, notes, comments or attachments; passwords,
tokens or secrets in any form; full IP addresses (truncated: IPv4 /24, IPv6 /48); AI prompts and
responses in clear text (metadata only: provider, model, purpose, scope).

**Masking of `changes`.** Three masking levels, derived from the six data classes of
[data-protection.md](./data-protection.md) §3 by `audit.MaskingFor`:

| Masking | Written as | Classes |
|---|---|---|
| `OPEN` | The value in clear (e.g. `OPEN → DONE`) | `NON_PERSONAL`, `PERSONAL_TECHNICAL` (already reduced where it is written — an address truncated, a user agent to a class) |
| `SENSITIVE` | "changed" plus a hash for comparability | `PERSONAL_BASIC`, `PERSONAL_CONTENT`, `SPECIAL_CATEGORY_RISK`, and any class this build does not recognise |
| `SECRET` | Not at all | `SECRET` |

A field with no classification is masked, never opened: gate PG-1 refuses one at build time and the
masking refuses one at run time. A trail that kept every title in full would undermine the deletion
obligation of the item it documents.

**One exception:** the **actor's label** (`PERSONAL_BASIC`) travels in clear (§2); after an erasure
the boundary answers it with a pseudonym (§6).

---

## 5. Access and analysis

| Role | Visibility |
|---|---|
| Tenant `OWNER`/`ADMIN` | The full audit trail of their own tenant |
| `MEMBER` | Their own events (`actor_id = self`) |
| Instance administrator (self-hosted/provider) | System-wide events; **no** blanket insight into tenant trails without a documented occasion, which is itself audited |
| Auditor | The `AUDITOR` role: the audit trail and the configuration, **no** content |

**The `AUDITOR` role** is not a rung on the ladder of the other six: it carries only `AUDIT_READ` and
`READ_CONFIGURATION`; somebody who needs more holds two memberships and the rights add up
(`core/domain/service.Allows`). `READ_CONFIGURATION`, split out of `STRUCTURE`
([domain-model.md](./domain-model.md) §3.2), reaches the configuration an auditor needs to judge an
entry — backup targets and runs, retention rules and previews, legal holds, automation rules and
runs, webhook subscriptions, the AI provider configuration — but no secret and no content.

**A renamed action stays one action.** A stored entry is never rewritten (§3); old entries keep the
old name, and the `action` filter matches both names and the family (`core/port/audit/Renamed.go`).

**Routes:** `GET /audit` with the shared query DSL (period, `action`, `actor`, `target`, `outcome`);
`POST /audit:export` as a signed JSON Lines or CSV archive with a checksum manifest and a stated
period; `POST /audit:verify` for the chain check. Scopes: `audit:read` for reading and verifying,
`audit:export` for the export — carrying a copy out is a different act.

Every audit export produces an audit entry; a **read** does not (the trail would grow by being
read), but a refused read does, and so does a verification that finds the chain broken
(`audit.chain_broken`, critical).

**The export is written in the clear**, and the manifest says so. "Signed" means the manifest's
digest sealed under the installation's master key and bound to that export — proof, to anybody who
can ask this installation, that it was produced here and not altered. An installation without a
key writes no signature and records that.

---

## 6. Auditability of the system (not just of its users)

Evidence a reviewer can check without taking the operator's word — technical evidence, not a
certification (ISO 27001 and SOC 2 are organisational):

| Evidence | Artefact |
|---|---|
| What is running here? | `product_version` in `GET /meta/capabilities` and the `hubtask_build_info` metric; a signed image with provenance |
| What is it made of? | An SBOM (CycloneDX) per release |
| Which controls are in force? | The pipeline's gate report per release (SG-1…SG-13, RT-1…RT-12), archived as an artefact |
| Which data is processed? | [data-catalog.md](../privacy/data-catalog.md), versioned in the repository |
| Which decisions were taken? | The ADRs, with date and status |
| Has restore been verified? | The restore drill record per release (RT-9) |
| Who has access? | An access review — memberships, roles, tokens, service accounts, last use. Planned: no route serves it yet |

### The audit's own life: pseudonymisation instead of deletion

**An erasure request does not reach the trail** — deleting the entries about a person would delete
the record that their request was handled. The trail is kept under the evidentiary interest for its
retention period (§1), holds no content, and cannot be edited in place (§3). So pseudonymisation
happens at the two points where it can:

* **At the boundary.** Once an account is erased, a read and an export of the trail answer the actor
  as a pseudonym derived per tenant from the identifier (`audit_pseudonym`). The row is untouched,
  the chain still verifies, and one actor's entries stay one actor's.
* **At the end of life.** The partition is dropped whole when the period is up (§3; not built yet).

The erasure itself is audited with `legal_basis = dsr.erasure`: the entry about the erasure is the
one that has to survive it.

### The instance's own journal: evidence where a per-tenant trail cannot live

The hard delete of a tenant ([multi-tenancy.md](./multi-tenancy.md) §5) removes the tenant's trail
by design. Its evidence, and that of every control-plane act, lives in **`instance_event`**: one row
per act (`tenant.provisioned`, `tenant.suspended`, `tenant.resumed`, `tenant.deletion_requested`,
`tenant.hard_deleted`, `tenant.password_opened`, `tenant.password_closed`,
`instance.session_elevated`). A row carries identifiers, the slug, the acting operator's label,
moments and counts (for a hard delete: rows, media objects and bytes, outbox events, queued jobs and
trail entries removed; for an opening of the password: its end, and that a requester and a reason
were given — never their texts, which stay in the workspace's own trail). Never content.

* **It belongs to no tenant** — no row policy ([multi-tenancy.md](./multi-tenancy.md) §2.1). Written
  only by the tenant lifecycle and instance use cases, append-only for the application role. The
  operator reads it through `GET /admin/journal`, newest first, behind `admin:tenants` and the
  operator register.
* **It commits with the act.** The hard delete writes the evidence, purges the trail through
  `purge_tenant_trail`, and lets the cascade take the rest, in one transaction.
* **It is not chained.** Here a chain would attest the operator to the operator; its integrity rests
  on the grants.

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
* Writes stay cheap: one insert, no foreign keys on actor or target, monthly partitions, per-tenant
  indexes on time, `seq`, action, actor and target only.

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

---

## 9. Open points

| # | Point | Needed by |
|---|---|---|
| A-3 | Establish the need for a SIEM connection (syslog/CEF export or pull through the API) | After `1.0.0` |

Closed points cited elsewhere: A-1 (the 400-day default) is §1; A-2 (external anchoring) is §3; A-4
(the auditor reads the configuration) is §5.
