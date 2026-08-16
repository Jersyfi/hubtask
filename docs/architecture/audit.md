# Audit Concept

Binding for all building blocks. Complements [arc42.md](./arc42.md) §8.14,
[ADR-0017](../adr/ADR-0017-audit-trail.md), [security.md](./security.md) (T-14, T-15), and
[data-protection.md](./data-protection.md).

---

## 1. Four separate kinds of record

The most common mistake in this area is a single "log" that mixes operations, product, and
evidence. Hubtask separates them strictly:

| Kind | Storage | Purpose | Audience | Mutable? | Retention |
|---|---|---|---|---|---|
| **Audit trail** | Table `audit_log` | Evidence of security- and privacy-relevant events | Auditor, data protection officer, tenant administrator | **No** (append-only, enforced by the database) | Configurable, 400 days by default |
| **Business history** | Table `activity_entry` | "Who moved this task?" — part of the product | End users | No (but deletable with the item) | The lifetime of the item |
| **Operational logs** | stdout/aggregator | Debugging | Operators | Ephemeral | 7–30 days |
| **Domain events** | `outbox_event` | Integration, automation | Systems | No | 7 days after delivery |

The audit trail is **not** a by-product of the events: events describe business changes, while the
audit trail also describes events *without* a business change — a failed login, read access to an
export, a rejected permission check. Those are exactly what an auditor asks about.

---

## 2. The structure of an audit entry

| Field | Contents |
|---|---|
| `id` | UUIDv7 (monotonic, time-sorted) |
| `tenant_id` | The tenant; system-wide events use a sentinel tenant |
| `occurred_at` | `timestamptz` (UTC), server time |
| `action` | A stable code, e.g. `item.deleted`, `member.role_changed`, `auth.login_failed`, `export.downloaded` |
| `outcome` | `SUCCESS` \| `DENIED` \| `FAILED` |
| `severity` | `INFO` \| `NOTICE` \| `WARNING` \| `CRITICAL` |
| `actor_type` | `USER` \| `SERVICE_ACCOUNT` \| `AUTOMATION` \| `AI_AGENT` \| `SYSTEM` |
| `actor_id`, `actor_label` | The ID and the label valid at the time (historised: the name stays readable even if the account is deleted later) |
| `on_behalf_of` | For automation/agents: the principal from `run_as` |
| `target_type`, `target_id`, `target_label` | The affected object |
| `context` | `request_id`, `trace_id`, `ip_truncated`, `user_agent_class`, `api_client`, `rule_id` |
| `changes` | A structured diff of **only the changed fields**, with values masked per field according to their classification (§4) |
| `legal_basis` | For privacy-relevant events: the legal basis or the occasion (e.g. `dsr.erasure`) |
| `prev_hash`, `hash` | The hash chain (§3) |
| `seq` | A tenant-local, gapless sequence number |

Actor details are stored **denormalised**. An audit entry that only points at a foreign key becomes
unreadable once the account is deleted — and an audit trail that loses its meaning through a
deletion does not do its job.

---

## 3. Immutability

Three levels, because one is not enough:

1. **No rights:** the application role `hubtask_app` holds only `INSERT` and `SELECT` on `audit_log` — no `UPDATE`, no `DELETE`, no `TRUNCATE`. Enforced by `GRANT`, checked by gate SG-4.
2. **Trigger lock:** a `BEFORE UPDATE OR DELETE` trigger raises an exception. Protection also against an operator who accidentally configured themselves too much power.
3. **Hash chain:** `hash = SHA-256(prev_hash ‖ canonical serialisation of the entry)`, one chain per tenant, plus a gapless `seq`. An endpoint `POST /audit:verify` checks the chain and reports the break point and any sequence gap.

A deliberate limit: the chain proves tampering **inside** the database, not against an attacker with
full database access who recomputes the entire chain. Anyone who needs that level exports the daily
chain end value (the "anchor") to an external, append-only target (a WORM bucket, a log service, a
signed email). There is a job and a documented configuration point for that — external anchoring is
optional and is not pretended to be in place.

Deletion happens exclusively through the retention job (age partitions), never individually. The
deletion itself produces an audit entry with the count and the period.

---

## 4. What gets audited — and what does not belong in it

**Mandatory events** (an extract; the full matrix is in `docs/audit/event-matrix.md`, generated from
the use case registry):

| Area | Events |
|---|---|
| Authentication | Login (success/failure/lockout), logout, MFA enabled/disabled, password change, refresh token reuse detected, step-up |
| Tokens | PAT created/revoked/first used, service account created, OAuth2 consent granted/withdrawn |
| Permissions | Role changed, invitation created/accepted/revoked, group changed, **denied access** (`outcome=DENIED`) |
| Tenant | Provisioning, suspension, settings change, deletion requested/executed |
| Data | Deletion, restore, trash emptied, archiving, bulk operation (with its scope), import |
| Export/access | Export requested, produced, **downloaded**, link expired; access to media marked sensitive |
| Automation | Rule created/changed/enabled/disabled, rule auto-disabled due to a loop or errors, outbound call blocked (SSRF) |
| AI | Agent action executed, AI processing with a third party, AI feature enabled/disabled |
| Integration | Webhook subscription created/changed, calendar feed created/revoked |
| Data protection | Data subject request created/fulfilled/rejected, retention period changed, anonymisation performed, data breach documented |
| Administration | Security-relevant configuration change, migration executed, audit retention changed |

**Not** in the audit trail: the content of tasks, notes, comments, or attachments; passwords,
tokens, or secrets in any form; full IP addresses (truncated: IPv4 /24, IPv6 /48); AI prompts and
responses in clear text (metadata only: provider, model, purpose, scope).

For `changes`, a **field classification** from the data catalogue applies: `OPEN` (the value in
clear text, e.g. a status change `OPEN → DONE`), `SENSITIVE` (only "changed" plus a hash for
comparability, e.g. a title), `SECRET` (not at all). This keeps the trail meaningful without it
becoming a data collection in its own right — an audit log that keeps every title change in full
text undermines the deletion obligation of the very item it documents.

---

## 5. Access and analysis

| Role | Visibility |
|---|---|
| Tenant `OWNER`/`ADMIN` | The full audit trail of their own tenant |
| `MEMBER` | Their own events (`actor_id = self`) — transparency towards the employee |
| Instance administrator (self-hosted/provider) | System-wide events; **no** blanket insight into tenant trails without a documented occasion, which is itself audited |
| Auditor | A dedicated `AUDITOR` role: read access to the audit trail and the configuration, **no** access to content |

The `AUDITOR` role exists because the alternative, in practice, is giving the auditor administrator
rights — a permissions problem that arises precisely where evidence is being demanded.

Access: `GET /audit` with the shared query DSL (filters on period, `action`, `actor`, `target`,
`outcome`), `POST /audit:export` as a signed JSON Lines or CSV archive with a checksum manifest and
a stated period, and `POST /audit:verify` for the chain check.
Every audit export itself produces an audit entry.

---

## 6. Auditability of the system (not just of its users)

"Auditable" also means that a reviewer can substantiate statements about the system without taking
the operator's word for it. The project supplies machine-readable evidence for that:

| Evidence | Artefact |
|---|---|
| What is running here? | `GET /meta/version` and `hubtask_build_info`: version, commit, build time; a signed image with provenance |
| What is it made of? | An SBOM (CycloneDX) per release |
| Which controls are in force? | The pipeline's gate report per release (SG-1…SG-12, RT-1…RT-12), archived as an artefact |
| Which data is processed? | [data-catalog.md](../privacy/data-catalog.md) — versioned in the repository, changes traceable through pull requests |
| Which decisions were taken? | The ADRs, with a date and a status |
| Has restore been verified? | A restore log per release (RT-9) |
| Who has access? | `GET /tenants/{id}/access-review`: all memberships, roles, tokens, service accounts, and last use — exportable as a periodic access review |

This replaces no certification (ISO 27001 and SOC 2 are organisational), but it creates the
technical evidence a reviewer would otherwise have to assemble laboriously. The access review is a
recurring obligation in both frameworks.

---

## 7. Architectural integration

* Auditing is **not an adapter concern.** The audit entry is created in the application layer within the same transaction as the business change: no event without an entry, no entry without an event. An adapter that audited on its own would miss events arriving through MCP or automation.
* The use case registry carries an `audit` attribute per use case (mandatory/optional, the `action` code, the target type, the classification). A use case with security or privacy relevance and **no** audit declaration fails the build (gate SG-13).
* Denied access is recorded in the `AuthorizationService` — the single place where authorisation happens. That makes `outcome=DENIED` complete without developers having to remember it.
* Audit writes matter for interactive latency: one insert, no foreign key checks on actor or target, monthly partitioning, and writing without index overkill (two indices: `(tenant_id, occurred_at DESC)` and `(tenant_id, action, occurred_at DESC)`).

---

## 8. Gates and tests

| ID | Check |
|---|---|
| SG-13 | Every use case with security or privacy relevance has an audit declaration (reconciled against the registry) |
| AT-1 | Test: `UPDATE`/`DELETE` on `audit_log` fails under the app role (both the grant **and** the trigger) |
| AT-2 | Test: the hash chain and `seq` are gapless after 1,000 mixed events; a tampered row is found by `:verify` |
| AT-3 | Test: denied access produces `outcome=DENIED` with the correct actor |
| AT-4 | Test: `changes` contains no `SENSITIVE` values in clear text and no secrets |
| AT-5 | Test: the audit entry and the business change are atomic (a rollback leaves no entry behind) |
| AT-6 | Test: the automation and MCP paths produce the same audit entries as the REST path (channel parity) |
| AT-7 | Test: deleting an account leaves audit entries readable (the denormalised `actor_label`) |

---

## 9. Open points

| # | Point | Needed by |
|---|---|---|
| A-1 | Agree the default audit trail retention period legally (evidentiary interest vs. storage limitation) | `0.6.0` |
| A-2 | The format and target of the external chain anchoring (WORM bucket, transparency log) | `0.9.0` |
| A-3 | Establish the need for a SIEM connection (syslog/CEF export or pull through the API) | After `1.0.0` |
