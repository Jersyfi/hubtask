# ADR-0018: Data protection by design — classification, a retention engine, no telemetry

* **Status:** accepted
* **Date:** 2026-08-14
* **Concerns:** data protection, data model, operations, product
* **Related:** [ADR-0010](./ADR-0010-multi-tenancy.md), [ADR-0012](./ADR-0012-ai-first-mcp.md), [ADR-0017](./ADR-0017-audit-trail.md), [data-protection.md](../architecture/data-protection.md)

## Context

Constraint C-12 requires GDPR conformance. The application runs in the EU, processes free text
content (which can unintentionally contain special categories under Art. 9), is multi-tenant, and is
meant to optionally integrate AI providers. GDPR Art. 25 requires data protection by design **and**
by privacy-friendly default.

Three properties can only be corrected later through data migration and therefore have to be decided
now: the discoverability of personal data across every storage location (deletion paths), the form
of retention periods (data or code), and the question of whether the software opens connections
outwards on its own.

In experience, a deletion concept does not fall apart at design time but at the twentieth new
feature: a new table is added, the deletion path is not considered, and the document no longer
matches the code — without anyone noticing.

## Decision

1. **Classification as a property of the model:** every field carries one of the classes `NON_PERSONAL`, `PERSONAL_BASIC`, `PERSONAL_CONTENT`, `PERSONAL_TECHNICAL`, `SPECIAL_CATEGORY_RISK`, `SECRET`. Unclassified fields with personal content fail the build (PG-1). The classification technically controls log output, audit masking, export scope, the deletion path, and the admissibility of passing data to third parties.
2. **The data catalogue as a versioned artefact** in the repository ([data-catalog.md](../privacy/data-catalog.md)), reconciled mechanically against the schema (PG-7) and verified in practice by a deletion test (PG-2). The catalogue is therefore verifiable rather than merely asserted.
3. **A retention engine:** retention periods are data (`retention_policy` per tenant and data kind) with documented bounds, evaluated by a scheduler job and logged in the audit. An extension beyond the default requires a justification and is audited.
4. **Data subject rights as core use cases** in the *Privacy & Compliance* bounded context, with a state machine, the statutory deadline, and a deadline alert (A-19) — not as a support process.
5. **Deletion with a choice:** anonymisation (authorship remains as "former user") or full deletion. The choice rests with the controller, because tenant data touches third parties' rights.
6. **No telemetry, no phone home.** Without explicit configuration, the application opens **no** outbound connection — not even to check for updates. Enforced by test PG-6 in a network sandbox.
7. **Third-country transfer requires confirmation:** an AI provider outside the EEA requires an explicit configuration confirmation; the use is audited with the provider, region, model, and purpose. The default for AI is off; the recommended path is a local model.
8. **Data residency** through `tenant.data_region` and regional cells on the basis of the existing shard routing — no distribution of individual records across tenants.
9. **Privacy by default** in every setting: truncated IP addresses, no third-party avatars, notifications without full content, the shortest defensible periods, sharing links expiring and not indexable.

## Options considered

| Option | Assessment |
|---|---|
| **Chosen: classification in the model + a verified catalogue + a retention engine** | A higher initial effort, but the only variant in which the deletion concept is still true after two years of feature work. |
| Data protection as a document with no technical anchoring | Common practice; it reliably drifts away from the code, and deletion gaps are discovered only during an incident. Rejected. |
| Periods in the code | Every change of period becomes a release, no tenant-specific adjustment, no evidence. Rejected. |
| End-to-end encryption as the data protection approach | The strongest protection, but incompatible with search, automation, and AI — it would be a different product. Rejected, documented in [security.md](../architecture/security.md) §15. |
| Full deletion as the only mode | Destroys context in third parties' tenant data (orphaned comments, an unclear history); anonymisation is offered in addition. |
| Anonymous usage statistics to the project (opt-out) | Useful for product decisions, but incompatible with the clarity that "the project is not the controller" and with the trust promise of a self-hosted application. Rejected. |
| AI processing as a standard feature | Would turn every installation into a transmission to third parties, potentially to a third country. Rejected; opt-in with a confirmation requirement. |

## Consequences

**Positive**

* Operators get the evidence they need for their record of processing activities, their TOM description, and data subject requests — without reading the code.
* Deletion paths stay complete, because new tables without a catalogue entry and a deletion path fail the build.
* A notification under Art. 33 is possible within 72 hours, because who is affected can be determined.
* The trust promise is verifiable: no data leaves without configuration, and that is tested.
* The CRA obligations of the later commercial variant are largely already met (SBOM, disclosure process, secure by default).

**Negative / countermeasures**

* *The classification obligation slows down the development of new fields.* → Default classes per field type; only deviations need a decision.
* *PG-2 (the deletion test across every storage location) is an expensive integration test.* → It runs nightly with test containers and covers the full matrix on every run; the highest protective value in the data protection area.
* *No telemetry means no usage data for product decisions.* → Replaced by voluntary feedback, public roadmap discussion, and metrics from our own hosted edition (with a legal basis and transparency there).
* *A confirmation-gated third-country transfer creates friction when setting up AI features.* → Intentional: the decision should be documented, not made by accident.
* *The retention engine is additional complexity in the scheduler.* → It uses the existing job infrastructure; one job type per data kind, per tenant, with a backlog metric.
* *Legal assessments remain open (P-1 … P-7).* → Explicitly tracked as open points; the architecture makes no legal claim.
