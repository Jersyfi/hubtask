# Data Protection and EU Compliance Concept

Binding for all building blocks ([ADR-0018](../adr/ADR-0018-privacy-by-design.md)). Every data
category — table, class, purpose, retention, deletion path, recipients — is in the
[data catalogue](../privacy/data-catalog.md); the TOM description is [tom.md](../privacy/tom.md).

> **Not legal advice.** Legal bases, contracts and third-country transfers belong with a lawyer or a
> data protection officer. Open points are in §12.

---

## 1. Why this is an architecture topic

GDPR Art. 25 (**by design and by default**) is a requirement on the data model. Three properties
cannot be retrofitted without a data migration:

1. **Deletability** — every piece of personal data has a known deletion path across *all* storage locations (database, object storage, search index, events, job queue, audit, backups).
2. **Retention periods as data, not as code** — otherwise every change of period is a release.
3. **Data residency** — regionalising a grown system afterwards is a rebuild.

---

## 2. Roles under data protection law

| Mode of operation | Controller (Art. 4(7)) | Processor (Art. 28) | Consequence for the architecture |
|---|---|---|---|
| Self-hosting by a private person (the household exemption, Art. 2(2)(c), may apply) | The person | None | Full features, no telemetry, nothing flowing to the project |
| Self-hosting by a company, association, or public body | The operator | None (without third-party services) | Evidence: the data catalogue, deletion features, the audit, a TOM description |
| Managed operation (a hosted service) | The customer (tenant) | The provider | A DPA, a sub-processor list, data residency, acting on instructions, deletion at contract end |
| AI with a third-party provider | The operator or the customer | The AI provider, as a sub-processor | Opt-in, provider and region chosen, audited, default **off** |

**The project is never a controller** — it ships software. Hence **no telemetry, no phone-home**:
unconfigured, the application contacts nothing, not even for updates (PG-6).

---

## 3. Data categories and classification

Every field carries one of six classes (`shared.DataClass`, the only vocabulary); which field has
which is the [data catalogue](../privacy/data-catalog.md) (Art. 30).

| Class | Meaning | Effect in the system |
|---|---|---|
| `NON_PERSONAL` | Configuration, enum values, counters | No restriction |
| `PERSONAL_BASIC` | Name, email, avatar, locale, time zone | Export, deletion, anonymisation, never logged |
| `PERSONAL_CONTENT` | Titles, notes, comments, attachments, activity history | As above, plus: never in logs, metrics, audit `changes`, or error messages |
| `PERSONAL_TECHNICAL` | IP address, user agent, session and device characteristics | Stored truncated, short retention |
| `SPECIAL_CATEGORY_RISK` | The product collects no special category, but free text can contain health or similar data (Art. 9) — "reschedule MRI appointment, oncology" | Free text goes to no third party (AI provider, external search index) without explicit activation; operators are pointed at their impact assessment |
| `SECRET` | Passwords, tokens, keys | Only hashed or encrypted, never exported, never audited |

**Masking in the audit trail** is derived from the class by `audit.MaskingFor` ([audit.md](./audit.md)
§4, which also states the one exception, the actor's label).

---

## 4. Data subject rights as use cases

Data subject rights are use cases with an API, auditing and deadline monitoring in the **Privacy &
Compliance** context, not manual support processes.

| Right | Use case | Implementation |
|---|---|---|
| Access (Art. 15) | `CreateDataSubjectRequest(ACCESS)` | A job writes the person's data from *every* tenant they belong to as a Hubtask archive to a backup target, with media and metadata (purpose, recipients, deadline) |
| Rectification (Art. 16) | Ordinary write operations | Appears in the audit |
| Erasure (Art. 17) | `CreateDataSubjectRequest(ERASURE)` | **`ANONYMIZE`** (authorship remains as "former user", the tenant's content is kept) or **`FULL_DELETE`** (the person's own contributions too), chosen by the controller. **A case that names no mode is carried out as `ANONYMIZE`** — it keeps what others depend on |
| Restriction (Art. 18) | `RestrictProcessing` | Account status `RESTRICTED`: readable, not processed, excluded from automation and AI |
| Portability (Art. 20) | `CreateDataSubjectRequest(PORTABILITY)` | JSON Lines + schema, not a PDF |
| Objection (Art. 21) | `WithdrawConsent`; a formal case as `CreateDataSubjectRequest(OBJECTION)` | Affects optional processing (AI, metering, notification channels); the core features stay usable |
| No automated individual decision-making (Art. 22) | — | AI results are only **suggestions** with provenance; automatic assignment has no legal effect, is overridable at any time and traceable in the audit |

**The case** is the `data_subject_request` table with a state machine
(`RECEIVED → IN_PROGRESS → COMPLETED | REJECTED`), the statutory deadline, an
assignee, a reason on rejection, and a deadline alert (`A-19`) — without one the right is missed in
practice. Rules:

* **The deadline is one calendar month from the day of receipt** (Art. 12(3); Reg. 1182/71 Art. 3):
  the day of receipt does not count, a month ending on a shorter month's last day is clamped, the
  deadline ends at the end of that day in the workspace's time zone, and it is never moved to the
  next working day, so the shown date is never later than the law's. Recording names the day of
  receipt; a case whose deadline has passed is recorded and shown overdue. Decided, not built: a
  case gets thirty days from when it is recorded today (#1196).
* **Moving a case to `IN_PROGRESS` starts the work**; the job completes the case. An erasure needs
  its mode and an export its target first; a case that cannot be carried out is refused at that
  step, not by a running job.
* **Permissions.** Recording, listing and refusing a case, and starting an export, need
  `MANAGE_MEMBERS`; starting an erasure needs `DELETE_CONTAINER` — it destroys work that belongs to
  the workspace as much as to the person.
* **`RESTRICTION` is a kind of request; `RESTRICTED` a state of an account**, which stands after the
  case closes. A restricted account still works; automatic processing stops, and
  `identity.AccountStatus.ProcessingAllowed` is the one predicate every such place asks.
* **An installation-wide case is a loop, not a wider query**: one workspace at a time under its own
  tenant context, through the ordinary repositories; only the tenant identifiers from
  `subject_tenants()` cross the boundary ([multi-tenancy.md](./multi-tenancy.md) §2.1). It needs
  `admin:tenants`; every workspace it touches gets its own audit entry. No repository method takes a
  tenant as an argument; a gate keeps that true.

### 4.1 Three decisions of 2026-09-30

**Decided** (milestone PH;
[`UC-PRV-01`](../usecases/privacy/UC-PRV-01-answer-a-data-subject-request-in-time.md),
[`UC-PRV-03`](../usecases/privacy/UC-PRV-03-erase-a-person-on-request.md),
[`UC-PRV-05`](../usecases/privacy/UC-PRV-05-withdraw-consent-to-optional-processing.md)); the
extension and the legal hold's reach are built; what a restriction keeps the kept data out of
(below) and the AI position not yet.

* **A deadline is extended once (Art. 12(3)).** An open case before its original deadline may be
  extended once, to at most three months after receipt, naming the reason (`COMPLEXITY` or
  `NUMBER_OF_REQUESTS`) and the date the person was informed, without which it is refused. Hubtask
  records that the controller told the person; it does not write to them. The watch and the register
  read the extended date, both dates stay visible, and the extension is audited
  (`dsr.extended`, with the reason and both dates, never the notes); an installation-wide case is
  extended by the operator and recorded in every workspace it touches.
  * **The bound is a calendar period in the workspace's own zone** (Reg. 1182/71 Art. 3(2)(c)):
    the day of receipt plus three months, or that month's last day where it has no such day —
    received 30 November, the last day is 28 February (29 in a leap year), never 2 March. The new
    deadline is later than the current one. Both inputs are days: the new deadline is the last
    second of the named day in the workspace's zone, and the informed day lies between the day of
    receipt and today there. A zone this installation cannot load counts as UTC; a zone changed
    since receipt counts as it is today. Receipt is `received_at` as recorded.
  * **Every workspace records it in its own trail.** In the extension's own transaction, one job
    per workspace the person's address is a member of (the case's own excepted) is queued; each
    writes `dsr.extended` there, naming the operator, in the same transaction that completes the
    job — so a retried run stores one entry. Active, suspended and pending-deletion workspaces all
    get theirs, a deleted one none; a case without an address names nobody elsewhere and queues
    nothing.
* **A legal hold wins over an erasure, exactly as far as it reaches (Art. 17(3)(e), Art. 18).** The
  erasure runs for everything no hold covers; what a hold covers is kept and *restricted* — out of
  automation and AI, but still in backups, the workspace export and the person's own Art. 15 copy,
  since restriction allows storage (Art. 18(2)) and a move must not lose what the hold keeps — and
  the case closes as partly completed, naming what was kept, under which hold and why (Art. 12(4)).
  Releasing the hold seeds the recorded remainder's erasure. An `ACCOUNT` hold covers the account
  and everything the person contributed to the workspace; it stops their erasure and deletion, not
  their sign-in.
  * **What each step keeps.** A row on an entry is kept when a hold covers the entry: the workspace,
    its hub or collection, the entry or one above it, or the account that created it - so the
    person's comments (in a full deletion), their assignments and the entries they created under it.
    An `ACCOUNT` hold keeps that account's own rows: the entries it created with their fields, its
    comments, its attached files, the account. The intake, matched by address, is kept only by a
    hold on the workspace. Credentials, notifications and unattached uploads go in every case: no
    hold reaches them. Several holds over one row: the oldest names it. One function decides it, and
    the erasure and its preview both call it.
  * **The account.** Only a hold on the person or on the workspace keeps the account itself - as it
    is, `RESTRICTED`, signing in, without a pseudonym. Under any other hold it is anonymised and
    pseudonymised exactly as without one, and the kept rows stay attributed to the former user; a
    full deletion that kept a row anonymises the account instead of removing it, and the rest
    removes it. Such a kept account's restriction cannot be lifted while the case keeps it.
  * **The record.** A partly completed case is `COMPLETED` and carries, per hold, what was kept
    (`erasure_kept`: counts, never content) and the legal basis, Art. 17(3)(e). The rows are written
    in the erasure's own transaction, under the case's row lock and the shared hold lock that every
    deletion takes before reading the holds; placing and lifting a hold take it exclusively.
  * **The rest.** Lifting a hold queues, in its own transaction, one job per case it kept part of;
    the job runs the same erasure against the holds in force, records what moved and writes
    `dsr.remainder_erased`. It is not stopped by the restriction its own erasure set. A full
    deletion whose kept person has come to run a rule waits, saying so, without a retry. The
    retention pass seeds every remainder nothing seeded - a release by an older binary, a restore
    that changed the holds. A kept entry moved out of its hold's reach goes with the next run of the
    rest, not at the move.
  * **Not yet built:** keeping what a hold keeps out of automation and AI (#1227).
* **AI: the workspace decides whether a person may keep their own content out.** Consent to AI
  processing stays the workspace's ([ADR-0072](../adr/ADR-0072-ai-at-the-installation-level.md));
  beside it the workspace takes one of two positions:
  * **Each person may keep their content out** (the default): what they authored goes into no
    prompt, their name becomes a neutral placeholder, and AI actions are not offered to them. The
    switch appears only where the workspace has AI on and more than one person.
  * **AI is part of the work for everybody**: on a basis not resting on each person's agreement
    (employment contract, works agreement, legal obligation) the workspace switches the opt-out off
    and must name that basis. People see the basis and whom to ask instead of a switch; a recorded
    withdrawal stops taking effect, and they are told so once. A formal Art. 21 objection stays
    possible as a data subject request.

  The installation and a plan may lock this **only towards the person** — never at *AI for
  everybody*, because only the controller can name a legal basis ([P-07](../vision/principles.md#p-07-the-stricter-level-wins-and-says-who-decided),
  [P-14](../vision/principles.md#p-14-ai-is-optional-and-nobody-consents-in-anothers-place)).
  Administrators see who objected in the consent register, and nowhere else.

---

## 5. Deletion concept and storage limitation

Every storage location of every data category is in the [data catalogue](../privacy/data-catalog.md)
with its retention and deletion path, verified by PG-2 and PG-7 against overlooked derived data
(risk R-09). The rules the catalogue does not state:

| Storage location | Rule |
|---|---|
| Primary tables | A cascading `DELETE` or an anonymising `UPDATE` |
| Search index (`tsvector`, optionally pgvector/external) | With the row, or by a reindex |
| `outbox_event` | Event payloads hold only references and `NON_PERSONAL` fields |
| `automation_rule` | Its `name` is the only free text. Deleting a rule is soft, so its runs stay accountable; `rule_run` bounds how long they are kept |
| `job` (an inbound webhook's payload) | An `INBOUND_WEBHOOK` body travels in the queue row as `payload`, bounded well below the request limit; it reaches no log, metric, trace, audit entry or job resource, and the queue's retention removes it |
| `rule_occurrence` | Goes with the rule or the entry, whichever goes first; holds no content and no person; not restored from an archive |
| `retention_rule` | Holds no content; not in a backup archive ([backup-restore.md](./backup-restore.md) §8.4). An entry's announcement (`retention_pending_until`, `retention_rule_id`, `retention_action`) goes with the entry |
| `audit_log` | **Not** individually deletable, so it holds no content ([audit.md](./audit.md) §4); an erasure is answered by a pseudonym ([audit.md](./audit.md) §6) |
| Operational logs | Without content and without clear-text identifiers |
| AI providers | A zero-retention agreement is a selection criterion; otherwise the provider is not approved ([ai-providers.md](../privacy/ai-providers.md)) |

**Backups: 35 days.** The operator's system backups and the PITR window are kept 35 days (thirty a
privacy policy can state, plus five so a monthly cycle cannot overrun it). A deletion reaches the
backups when the period has elapsed; data subjects are told. The storage enforces it: the cluster's
`retentionPolicy` ([`k8s/templates/cnpg-cluster.yaml`](../../k8s/templates/cnpg-cluster.yaml)) and
the backup bucket's Object Lock retention are both 35 days. Three rules follow:

* The system backup plan keeps **no** monthly or yearly generation.
* A point-in-time restore places the legal holds of the rewound period again, then re-applies its
  erasures, before traffic is admitted; a hold released in that period stays in force until its
  owner releases it again ([backup-restore.md](./backup-restore.md) §7, §8.5).
* Where the system backup is a `pg_dump` ([backup-restore.md §8.6](./backup-restore.md#86-the-minimal-path-a-dump-and-what-it-does-not-give)),
  the operator's own rotation must keep 35 days.

It does **not** bind a tenant's own archive backups. With the 30-day tenant grace period, a deleted
workspace is gone from everything the operator holds within 65 days of the request, unless a legal
hold stands:

* **A hold stops a workspace's deletion.** While any hold in the workspace is in force, a deletion
  request is refused, and a workspace already pending deletion stays pending until the last hold is
  lifted ([multi-tenancy.md](./multi-tenancy.md) §5).
* **A hold stops a destructive restore.** `REPLACE_TENANT` is refused while a hold is in force; one
  that runs keeps the workspace's own hold records rather than the archive's, so no hold is dropped
  or revived ([backup-restore.md](./backup-restore.md) §8.2).

Both refusals name the workspace as a whole, never which hold stands (P-01). A workspace pending
deletion takes no new hold, since its people are shut out and none could lift it; one that has a
hold anyway - from before this rule, or an older binary during an update - stays pending, its grace
job coming back daily, and leaves that state through the operator's *Cancel deletion*.

**Media reconciliation** runs per tenant, seeded by an upload's staging and rescheduling itself
([multi-tenancy.md](./multi-tenancy.md) §2.1). One pass:

1. **Recount and mark**, in one transaction: every live reference is recounted (the counter can
   drift) and what nothing points at is marked. Every read path refuses a marked object, so marking
   cannot be undone.
2. **Remove the bytes**, outside any transaction
   ([observability-reliability.md](./observability-reliability.md) §8); an object whose bytes will
   not go keeps its row for the next pass.
3. **Write the deletion journal entry and the tombstone and drop the row**, in one transaction, so a
   restore never brings back a file decided gone ([backup-restore.md](./backup-restore.md) §7).

Graces ([deployment.md](./deployment.md)): a confirmed object is an orphan only after pointing at
nothing for `HUBTASK_MEDIA_UNREFERENCED_GRACE` (1 hour, never zero; the recount stamps when a row
reached zero and clears it when a reference appears); an unconfirmed staged upload is abandoned after
`HUBTASK_MEDIA_STAGING_GRACE` (1 day); a marked object's bytes wait `HUBTASK_MEDIA_ORPHAN_GRACE`
(1 hour), so an operator can recover a mistaken removal.

**The retention engine** — rules as data, scoped to the workspace, a hub or a collection, with
bounds per data kind — is [data-retention.md](./data-retention.md); its defaults (§3 there) are the
shortest defensible periods (Art. 25(2)).

---

## 6. Data residency and third-country transfers

| Aspect | Implementation |
|---|---|
| Standard operation | All data in **one** region; the model forces no distribution. Self-hosting is data-local by construction |
| Provider operation | Planned, not built: the region as a tenant property, the shard routing of [multi-tenancy.md](./multi-tenancy.md) §2 leading to regional cells (an EU-only cell) |
| Outbound connections | Enumerated and each individually switchable ([data-catalog.md](../privacy/data-catalog.md) §6); **no** hidden one |
| AI providers | Opt-in per tenant, default off; a local model (Ollama) is recommended for privacy-sensitive installations; with a third-party provider, provider, region, model and purpose are audited |
| Third countries (Art. 44 ff.) | A provider declared `THIRD_COUNTRY` is refused at configuration unless the operator set `HUBTASK_AI_ALLOW_THIRD_COUNTRY_TRANSFER=true`. Adequacy, standard contractual clauses and the transfer impact assessment are the operator's ([ai-providers.md](../privacy/ai-providers.md)) |
| Webhook targets | An egress allowlist is mandatory in provider operation; target hosts are audited per tenant |

---

## 7. Other EU legal acts with architectural relevance

| Legal act | Relevance | Architectural provision |
|---|---|---|
| **NIS2** (Dir. 2022/2555) | Regulated operators must demonstrate security measures and reporting paths | Audit trail, access review, MFA enforcement, gate reports, the incident process ([security.md](./security.md) §14) |
| **Cyber Resilience Act** (Reg. 2024/2847, from late 2027) | Out of scope: Apache-2.0, not monetised, donations buy nothing ([ADR-0080](../adr/ADR-0080-hubtask-is-apache-2-0.md)); a paid offering is assessed before it appears. An operator building a commercial product on Hubtask may be a manufacturer under it | In place anyway: an SBOM per release, coordinated disclosure via `SECURITY.md`, signed artefacts, secure-by-default settings |
| **EU AI Act** (Reg. 2024/1689) | Supporting suggestions, not a high-risk system; transparency obligations apply | AI output marked as a suggestion with provenance, model and provider disclosed, the feature switchable, no automated decision with legal effect, AI use audited |
| **eIDAS / ePrivacy** | Cookies and tracking concern the frontend | Bearer tokens, not tracking cookies; no client sets a non-essential cookie without consent |
| **Data Act** (Reg. 2023/2854) | Switching and portability for data processing services | A complete tenant export in a documented format ([tenant-export.md](./tenant-export.md)), importers, no lock-in formats |
| **European Accessibility Act** | Concerns the frontend | WCAG 2.2 AA ([`design-system.md` §10](../design/design-system.md#10-accessibility)). The operator's accessibility statement is a legal link on the screens before sign-in, set among the installation's defaults; the health report names a multi-tenant installation without one. The project's own is under *About Hubtask* |

---

## 8. Technical and organisational measures (Art. 32)

The TOM description every operator needs is [`docs/privacy/tom.md`](../privacy/tom.md), derived
from [security.md](./security.md).

**Breach notification (Art. 33/34):** the audit trail and access logs evidence the affected
tenants, data categories and period within 72 hours
([`RB-GDPR-33`](../privacy/RB-GDPR-33-personal-data-breach.md), [tom.md](../privacy/tom.md) §8).

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

A task's readiness assesses the personal data it creates (class, legal basis, retention, deletion
path, recipients); it is done with the catalogue entry and the audit declaration. **Gates**, all in
`test/privacy/`:

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

PG-1, -3, -4, -5, -6 and -8 run in `make gate-privacy` (in `make verify` and every pull request; no
database). PG-2 (the real erasure) and PG-7 need a migrated database and run in
`make gate-privacy-full` — every pull request's data job and the nightly on arm64.
`make gate-selftest` proves each goes red against a deliberate violation; without a container
runtime the PG-2 and PG-7 probes report skipped, not passed. **PG-6 reads the source**, not the
network: every outbound call must name a target that arrived as configuration or as data
([ADR-0015](../adr/ADR-0015-security-baseline.md)).

---

## 11. Deliberately not included

| Not included | Reason |
|---|---|
| End-to-end encryption | Incompatible with search, automation, and AI ([security.md](./security.md) §15) |
| Finished legal documents (privacy policy, DPA) | A lawyer's work; the project supplies template skeletons and the technical details |
| Certifications | Organisational; the architecture creates the evidence |
| Automatic determination of the legal basis | The controller's assessment, not the software's |

---

## 12. Open points

| # | Point | Needed by |
|---|---|---|
| P-1 | Legal review of the data catalogue, the DPA template, and the privacy policy for a hosted service | Before commercial operation |
| P-2 | Data protection impact assessment (Art. 35) for a hosted service — whether it is required and how far it goes | Before `1.0.0` |
| P-7 | Whether a data protection officer must be appointed for a hosted service | Before commercial operation |
