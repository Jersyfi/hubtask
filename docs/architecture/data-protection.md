# Data Protection and EU Compliance Concept

Binding for all building blocks. Decision: [ADR-0018](../adr/ADR-0018-privacy-by-design.md).
Related: [audit.md](./audit.md), [security.md](./security.md),
[multi-tenancy.md](./multi-tenancy.md), [data-retention.md](./data-retention.md).

> **Not legal advice.** This document describes technical architecture decisions that make
> compliance easier. The legal assessment — legal bases, contracts, third-country transfers —
> belongs with a lawyer or a data protection officer. Open points are in §12.

---

## 1. Why this is an architecture topic

GDPR Art. 25 requires data protection **by design and by default** — a requirement on the data
model, not on documentation. Three properties cannot be retrofitted without a data migration:

1. **Deletability** — every piece of personal data has a known deletion path across *all* storage locations (database, object storage, search index, events, job queue, audit, backups).
2. **Retention periods as data, not as code** — otherwise every change of period is a release.
3. **Data residency** — regionalising a grown system afterwards is a rebuild, not a configuration change.

---

## 2. Roles under data protection law

| Mode of operation | Controller (Art. 4(7)) | Processor (Art. 28) | Consequence for the architecture |
|---|---|---|---|
| Self-hosting by a private person (the household exemption in Art. 2(2)(c) may apply) | The person themselves | None | Full features, no telemetry, no data flowing to the project |
| Self-hosting by a company, association, or public body | The operator | None (as long as no third-party services are used) | The operator needs evidence: the data catalogue, deletion features, the audit, a TOM description |
| Managed operation (a hosted service) | The customer (tenant) | The provider | A data processing agreement, a sub-processor list, data residency, acting on instructions, deletion at the end of the contract |
| AI enabled with a third-party provider | The operator or the customer | The AI provider, as a sub-processor | Opt-in, a choice of provider and region, transparency in the audit, default **off** |

**The project is never a controller** — it ships software, not a service. Hence one hard rule:
**no telemetry, no phone-home, no default connection to the outside.** Without explicit
configuration the application contacts nothing, not even to check for updates (gate PG-6).

---

## 3. Data categories and classification

The full record is [data-catalog.md](../privacy/data-catalog.md) (a record of processing activities
in the sense of Art. 30, versioned in the repository).

Every field carries one of six classes, the type `shared.DataClass`; there is no other vocabulary
of classes.

| Class | Meaning | Effect in the system |
|---|---|---|
| `NON_PERSONAL` | Configuration, enum values, counters | No restriction |
| `PERSONAL_BASIC` | Name, email, avatar, locale, time zone | Export, deletion, anonymisation, never logged |
| `PERSONAL_CONTENT` | Titles, notes, comments, attachments, activity history | As above, plus: never in logs, metrics, audit `changes`, or error messages |
| `PERSONAL_TECHNICAL` | IP address, user agent, session and device characteristics | Stored truncated, short retention |
| `SPECIAL_CATEGORY_RISK` | The product collects no special category, but free text can contain health or similar data (Art. 9) | A note to operators; heightened care with AI transmission and indexing |
| `SECRET` | Passwords, tokens, keys | Only hashed or encrypted, never exported, never audited |

**Masking in the audit trail** is derived from the class by `audit.MaskingFor`
([audit.md](./audit.md) §4). `NON_PERSONAL` and `PERSONAL_TECHNICAL` are recorded as they are
(technical data is already reduced where it is written — an address truncated, a user agent reduced
to a class). `PERSONAL_BASIC`, `PERSONAL_CONTENT` and `SPECIAL_CATEGORY_RISK` are recorded as
"changed" with a fingerprint. `SECRET` is not recorded. A class this build does not recognise is
treated as the middle one, never as clear text.

**One stated exception:** the **actor's label** is `PERSONAL_BASIC` and travels in the trail in clear,
because an entry that only points at a foreign key is unreadable once the account is deleted
([audit.md](./audit.md) §2). An erasure answers it with a pseudonym at the boundary instead
([audit.md](./audit.md) §6).

**`SPECIAL_CATEGORY_RISK`** exists because "reschedule MRI appointment, oncology" sits in a free text
field. Free text is therefore never sent to a third party (AI providers, an external search index)
without explicit activation, and the documentation points operators at their impact assessment.

---

## 4. Data subject rights as use cases

Data subject rights are core use cases with an API, auditing and deadline monitoring, in the
**Privacy & Compliance** bounded context — not manual support processes.

| Right | Use case | Implementation |
|---|---|---|
| Access (Art. 15) | `CreateDataSubjectRequest(ACCESS)` | A job writes a complete copy of the person's data across *every* tenant of the installation they are a member of to a backup target: a Hubtask archive with media and metadata (purpose, recipients, deadline) |
| Rectification (Art. 16) | Ordinary write operations | No special handling; the change appears in the audit |
| Erasure (Art. 17) | `CreateDataSubjectRequest(ERASURE)` | **Anonymisation** (`ANONYMIZE`: authorship remains as "former user", the tenant's content is kept) or **full deletion** (`FULL_DELETE`: the person's own contributions too). The controller chooses, because tenant data touches third parties' rights. **A case that names no mode is carried out as `ANONYMIZE`** |
| Restriction (Art. 18) | `RestrictProcessing` | Account status `RESTRICTED`: readable, not processed, excluded from automation and AI |
| Portability (Art. 20) | `CreateDataSubjectRequest(PORTABILITY)` | A machine-readable, documented format (JSON Lines + schema), not a PDF |
| Objection (Art. 21) | `WithdrawConsent`; a formal case as `CreateDataSubjectRequest(OBJECTION)` | Affects optional processing (AI, metering, notification channels); the core features stay usable |
| No automated individual decision-making (Art. 22) | — | AI results are only **suggestions** with provenance; automatic assignment is a work-organisation measure with no legal effect, overridable at any time and traceable in the audit |

**Why anonymisation is the default:** it keeps authorship, so a task somebody else depends on, a
comment in a thread and a decision with its reason survive the person leaving. Full deletion has the
wider blast radius, and a default is the setting nobody thinks about. A controller who owes maximal
erasure names `FULL_DELETE` on the case.

**The case** is the `data_subject_request` table with a state machine
(`RECEIVED → IN_PROGRESS → COMPLETED | REJECTED`), the statutory deadline (30 days by default), an
assignee, a reason on rejection, and a deadline alert (`A-19`). Rules:

* **Moving a case to `IN_PROGRESS` starts the work.** The archive or the erasure runs as a job, and
  the job completes the case; there is no second "run it" call. An erasure needs its mode resolved
  and an export needs its target before either starts; a case that cannot be carried out is refused
  at that step, not by a job already running.
* **Permissions.** Recording, listing and refusing a case, and starting an export, need
  `MANAGE_MEMBERS`. Starting an erasure needs `DELETE_CONTAINER`, because it destroys work that
  belongs to the workspace as much as to the person.
* **`RESTRICTION` is a kind of request; `RESTRICTED` is a state of an account.** The case closes once
  the restriction is in place, and the restriction stands. A restricted account still works —
  Art. 18 restricts what the controller does, not what the person may do. What stops is automatic
  processing: `identity.AccountStatus.ProcessingAllowed` is the one predicate every such place asks.
* **An installation-wide case is a loop, not a wider query.** One workspace at a time, under that
  workspace's own tenant context, through the ordinary repositories. What crosses the boundary is the
  list of tenant identifiers from `subject_tenants()` and nothing else
  ([multi-tenancy.md](./multi-tenancy.md) §2.1). It needs `admin:tenants`, and every workspace it
  touches gets its own audit entry, where that tenant's administrator can see it. No repository
  method takes a tenant as an argument; a gate keeps that true.

### 4.1 Three decisions of 2026-09-30

**Decided, not yet built** (milestone PH;
[`UC-PRV-01`](../usecases/privacy/UC-PRV-01-answer-a-data-subject-request-in-time.md),
[`UC-PRV-03`](../usecases/privacy/UC-PRV-03-erase-a-person-on-request.md),
[`UC-PRV-05`](../usecases/privacy/UC-PRV-05-withdraw-consent-to-optional-processing.md)). Until then
a legal hold on an `ACCOUNT` is refused ([data-retention.md](./data-retention.md) §4) and no
deadline can be extended.

* **A deadline is extended once, with a reason and a record that the person was told
  (Art. 12(3)).** An open case whose original deadline has not passed may be extended once, to at
  most three months after receipt, naming the reason — `COMPLEXITY` or `NUMBER_OF_REQUESTS` — and
  the date the person was informed; without that date the extension is refused. Hubtask does not
  write to the person; it records that the controller did. The watch and the register read the
  extended date; both dates stay visible; the extension is audited (`dsr.extended`). An
  installation-wide case is extended by the operator, and every workspace it touches records it.
* **A legal hold wins over an erasure, exactly as far as it reaches (Art. 17(3)(e), Art. 18).** The
  erasure runs for everything no hold covers. What a hold covers is kept and *restricted* — out of
  automation, AI and every export but the hold's own — and the case closes as partly completed,
  naming what was kept, under which hold and why (Art. 12(4)). The remainder is recorded on the
  case, and releasing the hold is the write that seeds its erasure. An `ACCOUNT` hold covers
  everything that person contributed to the workspace — entries they created, their comments,
  their attachments — and the account itself; it stops their erasure and deletion, not their
  sign-in.
* **AI: the workspace decides whether a person may keep their own content out.** The workspace's
  consent to AI processing stays the workspace's
  ([ADR-0072](../adr/ADR-0072-ai-at-the-installation-level.md)). Beside it the workspace takes one of
  two positions:
  * **Each person may keep their content out** (the default). *Keep my content out of AI* is an
    objection with effect: what the person authored — entries they created, their comments, their
    notes — goes into no prompt, their name is replaced by a neutral placeholder, and AI actions are
    not offered to them. The switch appears only where the workspace has AI on and more than one
    person.
  * **AI is part of the work for everybody.** For a workspace using AI on a basis that does not rest
    on each person's agreement (employment contract, works agreement, legal obligation), the
    workspace switches the personal opt-out off and must name that basis. People see a sentence
    naming the basis and whom to ask instead of a switch; a recorded withdrawal stops taking effect,
    and they are told so once. A formal objection under Art. 21 stays possible as a data subject
    request, which the controller decides.

  The installation and a plan may lock this **only towards the person**: they may hold a workspace
  at *each person may keep their content out*, never at *AI for everybody*, because only the
  controller can name a legal basis
  ([P-07](../vision/principles.md#p-07-the-stricter-level-wins-and-says-who-decided),
  [P-14](../vision/principles.md#p-14-ai-is-optional-and-nobody-consents-in-anothers-place)).
  Administrators see who objected in the consent register, and nowhere else.

---

## 5. Deletion concept and storage limitation

Every storage location of every data category is recorded in the data catalogue with a deletion
path, and a test verifies it (PG-2, PG-7) — the structural answer to overlooked derived data (risk
R-09).

| Storage location | Deletion path |
|---|---|
| Primary tables | A cascading `DELETE` or an anonymising `UPDATE` |
| Object storage (media) | Reference counting, then the reconciliation job (below) |
| Search index (`tsvector`, optionally pgvector/external) | With the row, or by a reindex |
| `outbox_event` | 7 days after delivery; event payloads hold only references and `NON_PERSONAL` fields |
| `webhook_delivery` | 30 days; request bodies stored truncated |
| `automation_rule` | With the tenant. Its `name` is the only free text; trigger, actions and parameters are identifiers and settings. Deleting a rule is soft, so its runs stay accountable; `rule_run` bounds how long they are kept |
| `rule_run` | 30 days; input only as a reference |
| `job` (an inbound webhook's payload) | With the job. An `INBOUND_WEBHOOK` delivery's body travels in the queue row so the run can read it as `payload`, bounded well below the request limit. It never reaches a log, metric, trace or audit entry; the job resource does not answer it; the queue's own retention removes it |
| `rule_occurrence` | With the rule or the entry, whichever goes first. Holds no content and no person. Not restored from an archive: each row is a debt owed at a moment in the source system's future |
| `retention_rule` | A primary table (cascading `DELETE`). Holds no content: a data kind, a period, an action, and the operator's own `justification`. Left out of a backup archive, because `EXPORT_THEN_DELETE` names an egress a restore does not recreate ([backup-restore.md](./backup-restore.md) §8.4). The announcement on an entry (`retention_pending_until`, `retention_rule_id`, `retention_action`) goes with the entry |
| `activity_entry` | With the item |
| `audit_log` | **Not** individually deletable, so it holds no content — only metadata and masked diffs ([audit.md](./audit.md) §4). An erasure records a pseudonym in `audit_pseudonym`; reads and exports answer the erased actor with it, the row is untouched and the chain verifies ([audit.md](./audit.md) §6) |
| Operational logs | 7–30 days, without content and without clear-text identifiers |
| Backups | **35 days** for the operator's system backups and point-in-time window (rules below) |
| AI providers | A zero-retention agreement is a selection criterion; otherwise the provider is not approved ([ai-providers.md](../privacy/ai-providers.md)) |

**Backups: 35 days.** The operator's system backups and the PITR window are kept 35 days — thirty a
privacy policy can state, plus five so a monthly cycle cannot overrun it. Deletion takes effect in
the primary system at once and in the backups when the period has elapsed; data subjects are told,
not left to discover it. The number is enforced by the storage: the database cluster's
`retentionPolicy` ([`k8s/templates/cnpg-cluster.yaml`](../../k8s/templates/cnpg-cluster.yaml)) and
the backup bucket's Object Lock retention are both 35 days. Three rules follow:

* The system backup plan keeps **no** monthly or yearly generation.
* A point-in-time restore that rewinds past a completed erasure re-applies the erasures from the
  rewound period before traffic is admitted ([backup-restore.md](./backup-restore.md) §7, §8.5).
* Where the system backup is a `pg_dump` ([backup-restore.md §8.6](./backup-restore.md#86-the-minimal-path-a-dump-and-what-it-does-not-give)),
  35 days is the promise the operator's own rotation must keep.

It does **not** bind a tenant's own archive backups: there the tenant is the controller and plans its
own generations. With the 30-day tenant grace period in front of it, a deleted workspace is gone from
everything the operator holds within 65 days of the request.

**Media reconciliation** runs per tenant, seeded by an upload's staging and rescheduling itself
([multi-tenancy.md](./multi-tenancy.md) §2.1). One pass has three parts:

1. **Recount and mark**, in one transaction: every live reference is recounted (the incremental
   counter can drift) and what nothing points at is marked. Every read path refuses a marked
   object, so marking cannot be undone.
2. **Remove the bytes**, outside any transaction (a bucket is an external dependency,
   [observability-reliability.md](./observability-reliability.md) §8). An object whose bytes storage
   will not release keeps its row and is retried next pass.
3. **Write the deletion journal entry and the tombstone and drop the row**, in one transaction, so a
   restore can never bring back a file this installation decided was gone
   ([backup-restore.md](./backup-restore.md) §7).

| Grace | Default | Meaning |
|---|---|---|
| `HUBTASK_MEDIA_UNREFERENCED_GRACE` | 1 hour | A confirmed object is an orphan only after pointing at nothing this long. The recount records when a row reached zero and clears it when a reference appears, so the window between upload and first use never orphans a file |
| `HUBTASK_MEDIA_STAGING_GRACE` | 1 day | A staged upload nobody confirmed is abandoned after this — long enough for a large file on a slow line |
| `HUBTASK_MEDIA_ORPHAN_GRACE` | 1 hour | A marked object's bytes wait this long, so an operator can still recover a mistaken removal |

Metrics: `hubtask_media_reclaimed_total`, `hubtask_media_reclaim_failed_total`.

**The retention engine** — rules as data, scoped to the workspace, a hub or a collection, with
bounds per data kind and a justification for exceeding the upper one — is
[data-retention.md](./data-retention.md). Its defaults are the shortest defensible periods
(Art. 25(2)): trash 30 days, `PERSONAL_TECHNICAL` 90 days, sessions 30 days, audit 400 days, rule
runs and webhook deliveries 30 days, notification history 90 days.

---

## 6. Data residency and third-country transfers

| Aspect | Implementation |
|---|---|
| Standard operation | All data in **one** region; the model forces no distribution. Self-hosting is data-local by construction |
| Provider operation | Planned: the region as a tenant property, with the shard routing of [multi-tenancy.md](./multi-tenancy.md) §2 as the path to regional cells (an EU-only cell). Neither exists yet |
| Outbound connections | Fully enumerated and each individually switchable: SMTP, object storage, OIDC, AI, external search index, webhook targets ([data-catalog.md](../privacy/data-catalog.md) §6). **No** hidden outbound connection |
| AI providers | Opt-in per tenant, default off; a local model (Ollama) is the recommended path for privacy-sensitive installations; with a third-party provider, the provider, region, model and purpose are recorded in the audit |
| Third countries (Art. 44 ff.) | A provider declared `THIRD_COUNTRY` is refused at configuration time unless the operator has set `HUBTASK_AI_ALLOW_THIRD_COUNTRY_TRANSFER=true` — deliberate friction, so the decision is documented rather than accidental. The adequacy decision or standard contractual clauses and the transfer impact assessment are the operator's ([ai-providers.md](../privacy/ai-providers.md)) |
| Webhook targets | An egress allowlist is mandatory in provider operation; target hosts are audited per tenant |

An enabled AI feature with a US provider turns a data-local installation into a third-country
transfer of personal free text. That is why it is a confirmation-gated, audited configuration step,
not a feature switch.

---

## 7. Other EU legal acts with architectural relevance

| Legal act | Relevance | Architectural provision |
|---|---|---|
| **NIS2** (Dir. 2022/2555) | Operators in regulated sectors must demonstrate security measures and reporting paths | Audit trail, access review, MFA enforcement, gate reports, the incident process in [security.md](./security.md) §14 |
| **Cyber Resilience Act** (Reg. 2024/2847, obligations from late 2027) | Concerns "products with digital elements" made available commercially. Free and open-source software supplied outside a commercial activity is outside its scope — where Hubtask stands: Apache-2.0, not monetised, funded by donations that buy nothing ([ADR-0080](../adr/ADR-0080-hubtask-is-apache-2-0.md)). A paid offering would be assessed again before it appears | The provision is in place anyway — an SBOM per release, coordinated disclosure via `SECURITY.md`, signed artefacts, documented secure-by-default settings. `SECURITY.md` states aims, not deadlines |
| **EU AI Act** (Reg. 2024/1689) | The AI features are supporting suggestions, not a high-risk system; transparency obligations still apply | AI output is always marked as a suggestion with provenance, model and provider are disclosed, the feature is switchable, no automated decision has legal effect, AI use is audited |
| **eIDAS / ePrivacy** | Cookies and tracking concern the frontend | Bearer tokens rather than tracking cookies; a binding client requirement: no non-essential cookies without consent |
| **Data Act** (Reg. 2023/2854) | Switching and portability obligations for data processing services | A complete tenant export in a documented format ([tenant-export.md](./tenant-export.md)), importers for third-party systems, no lock-in formats |
| **European Accessibility Act** | Concerns the frontend | WCAG 2.2 AA by criterion and the project's accessibility statement: [`design-system.md` §10](../design/design-system.md#10-accessibility). The duty to publish a statement falls on whoever provides the service — the operator: their statement is a legal link on the screens before sign-in, set among the installation's defaults, and the health report names a multi-tenant installation without one. The project's own statement is under *About Hubtask*. The backend delivers message codes, not text ([`i18n-l10n.md`](./i18n-l10n.md) §6) |

Even outside its scope the CRA matters: an operator who builds a commercial product on Hubtask may be
a manufacturer under it, and its obligations (SBOM, vulnerability handling, update period, secure by
default) cost little as a baseline and much as a retrofit.

---

## 8. Technical and organisational measures (Art. 32)

The TOM description every operator needs for their record of processing activities is
[`docs/privacy/tom.md`](../privacy/tom.md), derived from [security.md](./security.md): pseudonymisation
and encryption, tenant isolation through RLS, access control with RBAC and MFA, logging,
availability and recovery (RPO/RTO, verified restores), regular review (CI gates, access review,
pentest), and resilience.

**Breach notification (Art. 33/34):** the audit trail and access logs are designed so that the
affected parties can be *evidenced* — which tenants, which data categories, which period — within
72 hours. The procedure, with its queries, is
[`RB-GDPR-33`](../privacy/RB-GDPR-33-personal-data-breach.md). It lives beside the data catalogue,
not under `deploy/observability/runbooks/`, because a breach is noticed by a person and every
runbook there answers an alert.

---

## 9. Privacy by default — the concrete settings

| Setting | Default |
|---|---|
| AI processing | Off |
| A person's objection to AI for their own content | Offered; a workspace may withdraw the offer only on a named legal basis (§4.1, not yet built) |
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
| PG-4 | `PERSONAL_CONTENT` does not appear in logs, metrics, traces, audit `changes`, or error responses |
| PG-5 | The retention job deletes after expiry and logs it; periods outside the bounds are rejected |
| PG-6 | With no configuration, **no** outbound connection occurs |
| PG-7 | The data catalogue is consistent with the schema (every table/column with personal content is recorded) — a generated reconciliation |
| PG-8 | Third-country AI without explicit confirmation is refused |

PG-2 and PG-7 are the decisive ones: they keep the data catalogue from drifting away from the code
and stop deletion paths being forgotten when tables are added.

**Where each runs.** All live in `test/privacy/`:

| Gate | Runs in | Note |
|---|---|---|
| PG-1, PG-3, PG-4, PG-5, PG-6, PG-8 | `make gate-privacy`, part of `make verify` and of every pull request | They read the source and the declarations; no database, a second or two |
| PG-2, PG-7 | `make gate-privacy-full`, in every pull request's data job and in the nightly on arm64, with containers | Both need a migrated database; PG-2 also runs the real erasure |

`make gate-selftest` proves each one goes red against a deliberate violation. The probes for PG-2
and PG-7 are reported as skipped, not passed, where there is no container runtime; the nightly runs
them on a machine that has one.

**PG-6 is less than its name.** It reads the source for where a destination could come from rather
than sandboxing the network: every outbound call names a target that arrived as configuration or as
data ([ADR-0015](../adr/ADR-0015-security-baseline.md)). **PG-8** refuses at configuration time, and
`gate-selftest` proves it red both with the refusal removed and with the confirmation taken out of
the environment.

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
| P-1 | Legal review of the data catalogue, the DPA template, and the privacy policy for a hosted service | Before commercial operation |
| P-2 | Data protection impact assessment (Art. 35) for a hosted service — whether it is required and how far it goes | Before `1.0.0` |
| P-7 | Whether a data protection officer must be appointed for a hosted service | Before commercial operation |

Closed points cited elsewhere: P-3 (approved AI providers) is answered by
[ai-providers.md](../privacy/ai-providers.md); P-4 (CRA for a commercial variant) is moot under
Apache-2.0 (§7); P-5 (the 35-day backup period) is §5; P-6 (anonymisation as the erasure default) is
§4.
