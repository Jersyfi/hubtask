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

**What the hash is taken over is the entry as the row gives it back**, not as the caller built it,
and that is one rule with four edges — every one of which shipped broken and was found by driving
`hubctl audit verify` against a running installation (E-12):

* **The tail is the highest `seq`, never the newest timestamp.** Each caller reads its clock before
  it queues for the chain's per-tenant lock, so timestamps and sequence numbers disagree under
  concurrency. Reading the tail by time let two transactions continue from one number, and the
  unique index cannot catch it: a partitioned table's unique index has to carry the partition key,
  and `(tenant_id, occurred_at, seq)` lets one `seq` appear twice under two timestamps. The walk
  that verification uses is ordered by `seq` for the same reason.
* **The instant is truncated to what `timestamptz` keeps.** A clock offers nanoseconds and the
  column keeps microseconds, so an entry hashed as it arrived is not the entry that is read back.
* **The changed fields go through the reader's encoder before they are hashed.** A structure
  marshals in field order on the way in and in key order on the way out of `JSONB`; a retention
  plan or a restore report in an entry was enough to make a sound chain report tampering.
* **An entry that changed nothing hashes `{}`, not `null`.** A probe, a refusal, a recorded read:
  the row stores an empty object and reads back an empty object, and a nil map hashes as `null`. A
  trail verified for forty entries and broke at the first probe in it.

All four belong to the adapter, where the storage's precision and shape are known — the port and
the domain keep knowing nothing about PostgreSQL — and all four are held by tests that write
concurrently, with nanoseconds, with a structure in the changes, and with no changes at all.

A deliberate limit: the chain proves tampering **inside** the database, not against an attacker with
full database access who recomputes the entire chain. Anyone who needs that level exports the daily
chain end value (the "anchor") to an external, append-only target (a WORM bucket, a log service, a
signed email). There is a job and a documented configuration point for that — external anchoring is
optional and is not pretended to be in place.

Deletion happens exclusively through the retention job (age partitions), never individually. The
deletion itself produces an audit entry with the count and the period.

---

## 4. What gets audited — and what does not belong in it

**Mandatory events** (an extract; the full matrix is
[docs/audit/event-matrix.md](../audit/event-matrix.md), generated from the use case catalogue by
`make generate` — so it describes what this build records rather than what somebody remembered to
write down):

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
comparability, e.g. a title), `SECRET` (not at all).

These three are a *masking policy* rather than a fourth vocabulary of classes, and since E-11 they
are **derived** from the six of `data-protection.md` §3 by `audit.MaskingFor`: `NON_PERSONAL` and
`PERSONAL_TECHNICAL` are open, `PERSONAL_BASIC`, `PERSONAL_CONTENT` and `SPECIAL_CATEGORY_RISK` are
sensitive, `SECRET` is not recorded. A field written with no classification at all is masked rather
than opened — gate PG-1 refuses one at build time, and the masking refuses one at run time. This keeps the trail meaningful without it
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
rights — a permissions problem that arises precisely where evidence is being demanded. It is not a
rung on the same ladder as the other six: it carries the right to read the trail and none of the
rights every other role has, so somebody who needs both holds two memberships and the rights add up
rather than the stronger one winning (`core/domain/service.Allows`). What it grants today is the
trail; the configuration half of the row above is open point A-4.

Access: `GET /audit` with the shared query DSL (filters on period, `action`, `actor`, `target`,
`outcome`), `POST /audit:export` as a signed JSON Lines or CSV archive with a checksum manifest and
a stated period, and `POST /audit:verify` for the chain check. Two scopes rather than one:
`audit:read` for the first and the third, `audit:export` for the second, because reading the trail
and carrying a copy of it out of the installation are different acts.

Every audit export itself produces an audit entry. A *read* does not: the trail would grow by being
read, the second page would contain the reading of the first, and §4 does not list reading among the
mandatory events. What is recorded is the refusal — and a verification that finds the chain broken,
which is a critical entry of its own, because an auditor reading the trail months later has to be
able to see that somebody noticed.

The export is written in the clear, and the manifest says so. It is read by somebody outside this
installation — an auditor with a spreadsheet, a second system, a regulator — and encrypting it
under a key only this installation holds would make it unreadable exactly where it is meant to be
read. What "signed" means is stated in the archive itself: the manifest's digest sealed under the
installation's master key, bound to that export, which proves the archive was produced here and not
altered since to anybody who can ask this installation. An installation holding no key writes no
signature and records that rather than writing something that looks like one.

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

### The audit's own life: pseudonymisation instead of deletion

[data-retention.md](./data-retention.md) and [ADR-0020](../adr/ADR-0020-retention-policies.md) both
cite "audit.md §6" for this rule, and until E-09 it was in neither §6 nor anywhere else. It belongs
with the evidence rather than with the erasure, because what it protects is the evidence.

**An erasure request does not reach the trail.** Deleting the entries about a person would delete
the record that their request was handled — the one entry a supervisory authority would ask for.
The trail is kept under the evidentiary interest, for the period §9's open point A-1 has to settle
legally, and the entries about somebody are then a record of what was done, not a profile of them:
no content, a truncated address, a user agent class, no free text.

**It could not be edited in place even if it should be.** The application role holds no `UPDATE`, a
trigger refuses one, and every field an erasure would want to change — the actor's identifier, the
actor's label — is covered by the hash chain (§3). Rewriting a row to pseudonymise it would break
the chain of the tenant it belongs to, which destroys more evidence than the name it removed.

So pseudonymisation happens at the two points where it can:

* **At the boundary.** Once the account has been erased, what leaves the system carries no name: a
  read of the trail and an export answer the actor as a pseudonym derived per tenant from the
  identifier, rather than as the stored label. The row is untouched, the chain still verifies, and
  the entries about one actor are still one actor's — which is what an auditor needs and what a
  person's name is not needed for.
* **At the end of life.** When the retention period is up, the partition holding those entries is
  dropped whole (§3). That is the deletion, and it is the same one for everybody.

The erasure itself is an auditable action with `legal_basis = dsr.erasure`: an entry *about* the
erasure, which is exactly the entry that has to survive it. The boundary half is built by E-10,
which is where the erasure that makes it necessary is built; the rule is written here because two
documents were already pointing at it.

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

**One prefix, and what the other one meant.** This catalogue was `AT-n` here and `AU-n` in six other
documents, and the two did not map: `versioning-release.md` made AU-1 "every action marked auditable
produces exactly one entry", which is the declaration gate, while AT-1 here is the grants test. A
release criterion nobody can evaluate is not a criterion, so E-09 kept the prefix of the document
that defines the tests and rewrote the rest. The identifiers that were in circulation:

| Was | Is | Why |
|---|---|---|
| AU-1 ("every auditable action produces exactly one entry", "the audit registry") | **SG-13** | It is the declaration gate, reconciled against the registry at build time — a security gate rather than a test of the trail |
| AU-2 ("grants on `audit_log` checked") | **AT-1** | The same check, under the identifier this table gives it |
| AU-4 ("no user content in the audit") | **AT-4** | The same check |
| AU-1…AU-7 (as a set, in `roadmap.md` and `project-structure.md`) | **AT-1…AT-7** | The set this table defines |

---

## 9. Open points

| # | Point | Needed by |
|---|---|---|
| A-1 | Agree the default audit trail retention period legally (evidentiary interest vs. storage limitation) | `0.6.0` |
| A-2 | The format and target of the external chain anchoring (WORM bucket, transparency log) | `0.9.0` |
| A-3 | Establish the need for a SIEM connection (syslog/CEF export or pull through the API) | After `1.0.0` |
| A-4 | The `AUDITOR`'s second half: reading the configuration. The reads it would need sit behind `STRUCTURE`, which is a *writing* permission, so granting them means splitting a read-only configuration permission out of it — a change to the role matrix in `domain-model.md` §3.2 rather than to the audit surface | `0.5.0` |
