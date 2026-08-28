# Data Protection and EU Compliance Concept

Binding for all building blocks. Complements [arc42.md](./arc42.md) §8.15,
[ADR-0018](../adr/ADR-0018-privacy-by-design.md), [audit.md](./audit.md),
[security.md](./security.md), and [multi-tenancy.md](./multi-tenancy.md).

> **Not legal advice.** This document describes technical architecture decisions that make
> compliance easier. The legal assessment — in particular legal bases, contracts, and third-country
> transfers — belongs with a lawyer or a data protection officer. Open points are named in §12.

---

## 1. Why this is an architecture topic

GDPR Art. 25 requires data protection **by design and by default**. That is not a documentation
obligation but a requirement on the data model: anyone who does not build in deletability, purpose
limitation, and storage limitation from the start can only retrofit them through data migration.
Three decisions would be nearly impossible to correct later:

1. **Deletability** — every piece of personal data needs a known deletion path across *all* storage locations (database, object storage, search index, events, job queue, audit, backups).
2. **Retention periods as data, not as code** — otherwise every change of period is a release.
3. **Data residency** — regionalising a grown system afterwards is a rebuild, not a configuration change.

---

## 2. Roles under data protection law

| Mode of operation | Controller (Art. 4(7)) | Processor (Art. 28) | Consequence for the architecture |
|---|---|---|---|
| Self-hosting by a private person (the household exemption in Art. 2(2)(c) may apply) | The person themselves | None | Full features, no telemetry, no data flowing to the project |
| Self-hosting by a company, association, or public body | The operator | None (as long as no third-party services are used) | The operator needs evidence: the data catalogue, deletion features, the audit, a TOM description |
| Managed operation (hosted edition) | The customer (tenant) | The provider | A data processing agreement, a sub-processor list, data residency, acting on instructions, deletion at the end of the contract |
| AI enabled with a third-party provider | The operator or the customer | The AI provider, as a sub-processor | Opt-in, a choice of provider and region, transparency in the audit, default **off** |

**The project itself is never a controller in any of these constellations** — it ships software, not
a service. From which follows one hard rule: **no telemetry, no phone-home feature, no default
connection to the outside.** Without explicit configuration, the application makes no contact with
the project's systems — not even to check for updates.

---

## 3. Data categories and classification

The full record: [data-catalog.md](../privacy/data-catalog.md) (a record of processing activities in
the sense of Art. 30, versioned in the repository).

Every field in the model carries a classification, held machine-readably in the
`CustomFieldDefinition` or in the metadata of the domain models. Since E-11 the six classes are a
type - `shared.DataClass` - rather than a list in a document, because three vocabularies had grown
where there should have been one: this table's six, the data catalogue's five (it had lost
`SPECIAL_CATEGORY_RISK`), and `audit.md` §4's three, which are not classes at all but a *masking*
policy. There is one vocabulary and one derivation now, and both are code a gate can read.

| Class | Meaning | Effect in the system |
|---|---|---|
| `NON_PERSONAL` | Configuration, enum values, counters | No restriction |
| `PERSONAL_BASIC` | Name, email, avatar, locale, time zone | Export, deletion, anonymisation, never logged |
| `PERSONAL_CONTENT` | Titles, notes, comments, attachments, activity history | As above, plus: never in logs, metrics, audit `changes`, or error messages |
| `PERSONAL_TECHNICAL` | IP address, user agent, session and device characteristics | Stored truncated, short retention |
| `SPECIAL_CATEGORY_RISK` | The product deliberately collects no such category, but free text can contain health or similar data (Art. 9) | A note to operators in the documentation; heightened care with AI transmission and indexing |
| `SECRET` | Passwords, tokens, keys | Only hashed/encrypted, never exported, never audited |

**The derivation to the trail's masking** (`audit.MaskingFor`, `audit.md` §4): `NON_PERSONAL` and
`PERSONAL_TECHNICAL` are recorded as they are, because technical data is already reduced where it
is written - an address truncated to a prefix, a user agent to a class. `PERSONAL_BASIC`,
`PERSONAL_CONTENT` and `SPECIAL_CATEGORY_RISK` are recorded as "changed" with a fingerprint.
`SECRET` is not recorded at all. A class this build does not recognise is treated as the middle
one, because guessing "record it in clear text" is the one direction that cannot be taken back.

There is one stated exception, and it is stated rather than derived: the **actor's label** is
`PERSONAL_BASIC` and travels in the trail in clear, because an entry that only pointed at a foreign
key becomes unreadable once the account is deleted ([audit.md](./audit.md) §2). An erasure answers
it with a pseudonym at the boundary instead (§6).

The `SPECIAL_CATEGORY_RISK` class is named deliberately: a task manager collects no health data —
but "reschedule MRI appointment, oncology" sits in a free text field. That is why free text content
is never transmitted to third parties (AI providers, an external search index) without explicit
activation, and why the documentation points operators at their impact assessment obligation.

---

## 4. Data subject rights as use cases

Data subject rights are **not** manual support processes but core use cases with an API, auditing,
and deadline monitoring. They live in the **Privacy & Compliance** bounded context.

| Right | Use case | Implementation |
|---|---|---|
| Access (Art. 15) | `CreateDataSubjectRequest(ACCESS)` | An asynchronous job produces a complete copy of all the person's data across *every* tenant of the installation in which they are a member; structured JSON plus media plus metadata (purpose, recipients, deadline) |
| Rectification (Art. 16) | Ordinary write operations | No special handling needed, the change appears in the audit |
| Erasure (Art. 17) | `CreateDataSubjectRequest(ERASURE)` | Two-stage: **anonymisation** (authorship remains as "former user", the tenant's content is preserved) or **full deletion** including the person's own comments — the choice rests with the controller, because tenant data can belong to third parties |
| Restriction (Art. 18) | `RestrictProcessing` | Account status `RESTRICTED`: readable, not processed, excluded from automation and AI |
| Portability (Art. 20) | `CreateDataSubjectRequest(PORTABILITY)` | A machine-readable, documented format (JSON Lines + schema), not just a PDF |
| Objection (Art. 21) | `WithdrawConsent` | Affects optional processing (AI, metering, notification channels); the core features stay usable |
| No automated individual decision-making (Art. 22) | — | AI results are exclusively **suggestions** with provenance; automatic assignment is a work-organisation measure with no legal effect, overridable at any time and traceable in the audit |

Carried technically by the `data_subject_request` table with a state machine
(`RECEIVED → IN_PROGRESS → COMPLETED | REJECTED`), the statutory deadline (30 days by default), an
assignee, a reason on rejection, and a deadline alert (`A-19`) as it approaches. Without deadline
monitoring, the right gets violated in practice even though the feature exists.

Built with E-10, and four things about it are decisions rather than mechanics:

* **Moving a case to `IN_PROGRESS` is what starts the work.** The archive or the erasure runs as a
  job and the job completes the case; there is no second call to "run it", which would be a state
  machine with a state outside itself. An erasure needs its mode and an export needs its target
  before either can start - a case that cannot be carried out is refused at the step that would
  have started it rather than by a job that has already begun.
* **Starting an erasure asks for the owner's right** (`DELETE_CONTAINER`), because it destroys work
  that belongs to the workspace as much as to the person. Recording, listing and refusing a case are
  the administrator's (`MANAGE_MEMBERS`); starting an *export* is too, because it writes to a target
  somebody has already approved.
* **`RESTRICTION` is a kind of request; `RESTRICTED` is a state of an account.** The case is closed
  once the restriction is in place, and the restriction goes on standing. A restricted account
  still works - Art. 18 restricts what the controller does with the data, not what the person may
  do - and what stops is automatic processing: `identity.AccountStatus.ProcessingAllowed` is the one
  predicate every such place asks, and the assignment policy is the first that does.
* **An installation-wide case is a loop rather than a wider query.** One workspace at a time, under
  that workspace's own tenant context, through the ordinary repositories; what crosses the boundary
  is a list of tenant identifiers from one `SECURITY DEFINER` function and nothing else. It needs
  the `admin:tenants` scope, and every workspace it touches gets its own audit entry - because
  `audit.md` §5 says an instance administrator has no blanket insight into a tenant's data without a
  documented occasion, and the occasion belongs where that tenant's own administrator can see it.
  No repository method takes a tenant as an argument, which a gate now keeps true.

---

## 5. Deletion concept and storage limitation

Risk R-09 (overlooked derived data) is addressed structurally: for every data category, *every*
storage location is recorded in the data catalogue with a deletion path, and a test verifies it.

| Storage location | Deletion path |
|---|---|
| Primary tables | A cascading `DELETE` or an anonymising `UPDATE` |
| Object storage (media) | Reference counting, then a deletion request; orphaned objects via a reconciliation job (see below) |
| Search index (`tsvector`, optionally pgvector/external) | With the row, or via a reindex request |
| `outbox_event` | Short period (7 days after delivery); event payloads limited to references and `NON_PERSONAL` fields |
| `webhook_delivery` | 30 days; request bodies stored truncated |
| `automation_rule` | With the tenant. A rule's `name` is a title somebody wrote and is the only free text on the row - the trigger, the actions and their parameters are identifiers and settings. Deleting a rule is soft, so that the runs it produced stay accountable; the row goes with the workspace, and the `rule_run` row below bounds how long what it did is kept |
| `rule_run` | 30 days; input data only as a reference |
| `activity_entry` | With the item |
| `audit_log` | **Not** individually deletable; therefore contains no content, only metadata and masked diffs (see [audit.md](./audit.md) §4). An erasure records a pseudonym in `audit_pseudonym` instead, and every read and every export answers the erased actor's label with it - the row is untouched, the chain still verifies, and what leaves carries no name (§6, E-10) |
| Operational logs | 7–30 days, without content and without clear-text identifiers |
| Backups | The retention period is documented; deletion takes effect once the backup cycle has elapsed — a fact that is made transparent to the data subject rather than concealed |
| AI providers | A zero-retention agreement as a selection criterion; otherwise the provider is not approved |

**The media reconciliation** (C-06) is that job, and it runs per tenant like the retention sweep,
seeded by a staging rather than by a scheduler — nothing may enumerate tenants
([multi-tenancy.md](./multi-tenancy.md) §2.1). One pass is three parts, and they are three because
one of them may not be inside a transaction. It recounts every live reference and marks what
nothing points at — both in one transaction, because a count read in one and acted on in another is
a count something can move in between, and the recount exists precisely because the incremental
counter can drift. It then removes the bytes, outside any transaction, since a bucket is an
external dependency ([observability-reliability.md](./observability-reliability.md) §8). Finally it
writes the deletion journal entry and the tombstone and drops the row, all three in one
transaction, so that a restore from backup can never bring back a file this installation decided
was gone ([ADR-0020](../adr/ADR-0020-retention-policies.md) §6).

Three graces bound it, and the first of them is the one that matters most, because marking is not a
reversible step: every read path refuses a marked object, so nothing can attach it, so no recount
will ever see a reference on it again. A confirmed object that points at nothing is therefore an
orphan only once it has pointed at nothing for `HUBTASK_MEDIA_UNREFERENCED_GRACE` (an hour by
default) — never merely because a pass caught it between its confirmation and the first thing that
uses it, which is a window every upload passes through and every detachment opens again. The
recount is what records when a row reached zero, keeping the first such moment and clearing it when
a reference appears. A staged upload nobody confirmed is abandoned after
`HUBTASK_MEDIA_STAGING_GRACE` (a day), so a large file still travelling up a slow line is not
mistaken for one. A marked object then waits out `HUBTASK_MEDIA_ORPHAN_GRACE` (an hour) before its
bytes go, which is the window in which an operator who notices a mistaken removal still finds them
where they were. An object whose bytes storage will not release keeps
its row and is tried again next pass — the other order would leave a file in the bucket that nothing
in this system knows about any more. Metrics: `hubtask_media_reclaimed_total`,
`hubtask_media_reclaim_failed_total`.

**The retention engine:** the rules are data (`retention_rule`, scoped to the workspace, a hub or a
collection), not code; `retention_policy` carries the bounds an operator sets per data kind. A
scheduler job evaluates them tenant by tenant, records the scope in the audit, and reports backlogs
as a metric. Configurable within documented lower and upper bounds; an extension beyond the upper
bound produces an audit entry with a justification field.

`retention_rule` holds no content of anybody's: a class of data, a number of days, an action, and
`justification` - which is the operator's own words about their own policy rather than a person's
data. It is a primary table, so its deletion path is the cascading `DELETE` of the table above, and
it is deliberately left out of a backup archive: a rule that says `EXPORT_THEN_DELETE` names an
egress channel a restore does not recreate ([backup-restore.md](./backup-restore.md) §8.4). The
announcement a rule leaves on an entry - `retention_pending_until`, `retention_rule_id`,
`retention_action` - goes with the entry, and says nothing a reader of the entry could not already
see.

Defaults (privacy-friendly, Art. 25(2)): trash 30 days, `PERSONAL_TECHNICAL` 90 days, sessions 30
days, audit 400 days, rule runs and webhook deliveries 30 days, notification history 90 days.

---

## 6. Data residency and third-country transfers

| Aspect | Implementation |
|---|---|
| Standard operation | All data in **one** region; the model forces no distribution. Self-hosting is data-local by construction. |
| Provider operation | The region is a tenant property (`tenant.data_region`); the shard routing described in [multi-tenancy.md](./multi-tenancy.md) is the technical path to regional cells (an EU-only cell is possible). |
| Outbound connections | Fully enumerated and each individually switchable: SMTP, object storage, OIDC, AI, external search index, webhook targets. There is **no** hidden outbound connection. |
| AI providers | Opt-in per tenant, default off; the adapter for local models (Ollama) is the recommended path for EU privacy-sensitive installations; with third-party providers, the provider, region, model, and purpose are recorded in the audit |
| Third countries (Art. 44 ff.) | Selecting a provider outside the EEA requires an explicit confirmation in the configuration (`HUBTASK_AI_ALLOW_THIRD_COUNTRY_TRANSFER=true`) — deliberate friction, so the decision is documented rather than made by accident; the documentation names the adequacy decision or standard contractual clauses and the transfer impact assessment as the operator's obligation |
| Webhook targets | An egress allowlist is mandatory in provider operation; target hosts are audited per tenant |

This point deserves emphasis: an enabled AI feature with a US provider turns a data-local
installation into a third-country transfer of personal free text content. Which is why it is not
merely a feature switch but a confirmation-gated, audited configuration step.

---

## 7. Other EU legal acts with architectural relevance

| Legal act | Relevance | Architectural provision |
|---|---|---|
| **NIS2** (Dir. 2022/2555) | Operators in regulated sectors must demonstrate security measures and reporting paths | Audit trail, access review, MFA enforcement, gate reports, the incident process in [security.md](./security.md) §14 |
| **Cyber Resilience Act** (Reg. 2024/2847, obligations from late 2027) | Concerns "products with digital elements" made available commercially. Pure open source provision without commercialisation is exempt — a **commercial offering** (hosted edition, commercial licences per [ADR-0013](../adr/ADR-0013-licensing.md)) falls within scope | An SBOM per release, coordinated disclosure via `SECURITY.md`, vulnerability handling with deadlines, security updates over a defined support period, signed artefacts, documented secure-by-default settings — all already in place, named here deliberately as CRA provision |
| **EU AI Act** (Reg. 2024/1689) | The AI features are supporting suggestions, not a high-risk system; transparency obligations are nonetheless relevant | AI output is always marked as a suggestion with provenance, the model and provider are disclosed, the feature is switchable, there is no automated decision with legal effect, and AI use is logged in the audit |
| **eIDAS / ePrivacy** | Cookies and tracking primarily concern the frontend | The backend uses bearer tokens rather than tracking cookies; a binding client requirement: no non-essential cookies without consent |
| **Data Act** (Reg. 2023/2854) | Switching and portability obligations for data processing services | A complete tenant export in a documented format, importers for third-party systems, no lock-in formats |
| **European Accessibility Act** | Concerns the frontend | A binding client requirement (WCAG 2.2 AA) in the roadmap; the backend delivers message codes rather than text |

The CRA point is strategically relevant: it hits precisely the commercial variant the licence model
provides for — and its obligations (SBOM, vulnerability handling, update period, secure by default)
are expensive to retrofit but nearly free as a baseline from the start.

---

## 8. Technical and organisational measures (Art. 32)

The TOM description that every operator needs for their record of processing activities ships as
[`docs/privacy/tom.md`](../privacy/tom.md), derived from [security.md](./security.md), and names:
pseudonymisation and encryption (at rest and in transit), tenant isolation through RLS, access
control with RBAC and MFA, logging, availability and recovery measures (RPO/RTO, verified restores),
procedures for regular review (CI gates, access review, pentest), and resilience (the resilience
patterns).

**Breach notification (Art. 33/34):** the audit trail and access logs are designed so that affected
parties can be *evidenced* — which tenants, which data categories, which period. Without that
analysability, a notification within 72 hours is not possible. The runbook
[`RB-GDPR-33`](../privacy/RB-GDPR-33-personal-data-breach.md) describes the procedure including the
queries. It lives beside the data catalogue rather than under
`deploy/observability/runbooks/`, because nothing fires it: a breach is noticed by a person, and
every runbook in that directory answers an alert.

---

## 9. Privacy by default — the concrete settings

| Setting | Default |
|---|---|
| AI processing | Off |
| External search index | Off (PostgreSQL full text is data-local) |
| Telemetry / usage statistics sent to the project | Does not exist |
| Metering (usage figures for billing) | Off; when enabled, aggregates only, no content |
| Public sharing links | Off, expiring, revocable, not indexable |
| Avatar retrieval from third-party services (Gravatar and similar) | Off (it leaks IP addresses to third parties) |
| Full IP addresses in logs | No, truncated |
| Email notifications containing task content | Title and link only, no full text; switchable |
| Visibility of profile data to other tenant members | Minimal (display name, avatar) |
| Retention | The shortest defensible periods (§5) |

---

## 10. Integration into development and CI

* The **Definition of Ready** includes the data protection assessment: which personal data arises, its classification, the legal basis, retention, the deletion path, recipients.
* The **Definition of Done** requires the data catalogue entry and the audit declaration.
* **Gates:**

| ID | Check |
|---|---|
| PG-1 | Every field with personal content has a classification; unclassified fields fail the build |
| PG-2 | Deletion test: after erasure of a person, **no** storage location (database, object storage, search index, outbox, rule runs, deliveries) still holds personal data — apart from the permitted audit metadata |
| PG-3 | Export completeness test: the access export contains every field classified as personal (catalogue reconciled against the export schema) |
| PG-4 | Test: `PERSONAL_CONTENT` does not appear in logs, metrics, traces, audit `changes`, or error responses |
| PG-5 | Test: the retention job deletes after expiry and logs it; periods outside the bounds are rejected |
| PG-6 | Test: with no configuration, **no** outbound connection occurs (network sandbox in the test) |
| PG-7 | The data catalogue is consistent with the schema (every table/column with personal content is recorded) — a generated reconciliation |
| PG-8 | Test: third-country AI without explicit confirmation is refused |

PG-2 and PG-7 are the decisive ones: they prevent the data catalogue from drifting away from the
code and stop deletion paths being forgotten when new tables are added — the usual route by which a
clean deletion concept becomes untrue over two years.

**Where each of them runs** (E-11). They live in `test/privacy/`, and the split is by cost:

| Gate | Runs in | Note |
|---|---|---|
| PG-1, PG-3, PG-4, PG-5, PG-6, PG-8 | `make gate-privacy`, part of `make verify` and of every pull request | They read the source and the declarations; no database, a second or two |
| PG-2, PG-7 | `make gate-privacy-full`, in the nightly with containers | Both need a migrated database; PG-2 additionally runs the real erasure |

Each one is proved to go red by `make gate-selftest` against a deliberate violation — which is what
distinguishes a gate from a table like this one. The probes for PG-2 and PG-7 are skipped where
there is no container runtime and reported as skipped rather than passed, and the nightly runs the
whole script on the machine that has one.

Two of them are less than they look, and say so where they run. PG-6 reads the source for addresses
rather than sandboxing the network: what a build can decide is where a destination could come from,
and every outbound call this system makes names a target that arrived as configuration or as data
([ADR-0015](../adr/ADR-0015-security-baseline.md)). PG-8 has nothing to refuse — there is no AI
provider surface in this build — so it is a tripwire that fires when one arrives, and the measure
belongs to `0.7.0`.

---

## 11. Deliberately not included

| Not included | Reason |
|---|---|
| End-to-end encryption | Incompatible with search, automation, and AI; see [security.md](./security.md) §15 |
| Finished legal documents (privacy policy, data processing agreement) | These must be drawn up by a lawyer; the project supplies template skeletons and the technical details that belong in them |
| Certifications | Organisational; the architecture creates the evidence |
| Automatic determination of the legal basis | An assessment for the controller, not for the software |

---

## 12. Open points

| # | Point | Needed by |
|---|---|---|
| P-1 | Legal review of the data catalogue, the DPA template, and the privacy policy for the hosted edition | Before commercial operation |
| P-2 | Data protection impact assessment (Art. 35) for the hosted edition — clarify whether it is required and how far it goes | Before `1.0.0` |
| P-3 | Selection of approved AI providers including zero-retention evidence and an EEA region | `0.7.0` |
| P-4 | Have legal advice assess CRA applicability to the commercial variant; set the support period per release | 2027 (preparatory work from `1.0.0`) |
| P-5 | Backup retention vs. the deletion obligation: fix and document the period bindingly | `0.6.0` |
| P-6 | Decide anonymisation vs. full deletion as the tenant default | `0.6.0` |
| P-7 | Assess whether a data protection officer must be appointed for the hosted edition | Before commercial operation |
