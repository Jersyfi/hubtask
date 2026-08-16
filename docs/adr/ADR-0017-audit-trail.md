# ADR-0017: An immutable audit trail, separate from the business history

* **Status:** accepted
* **Date:** 2026-08-14
* **Concerns:** audit, security, data protection, persistence
* **Related:** [ADR-0007](./ADR-0007-events-outbox-cloudevents.md), [ADR-0015](./ADR-0015-security-baseline.md), [audit.md](../architecture/audit.md)

## Context

The system must be auditable: an auditor, a data protection officer, or a tenant administrator must
be able to evidence who did what and when — including events without a business change (a failed
login, a rejected permission check, a downloaded export). At the same time, three kinds of record
already exist: the user-visible activity history (`activity_entry`), domain events
(`outbox_event`), and operational logs.

The obvious simplification would be to derive the audit trail from the domain events. It does not
work: events describe only successful state changes, have short retention, are designed for
integration rather than evidence, and know nothing about denied access. Equally obvious — and
equally wrong — would be using the activity history: it is deleted with the item, precisely when it
would be needed as evidence.

On top of that, two goals are in tension: the audit trail should be meaningful, but the GDPR
deletion obligation must not be undermined by a log that keeps content permanently.

## Decision

1. **A dedicated `audit_log` table**, conceptually and technically separate from `activity_entry`, `outbox_event`, and operational logs; four kinds of record with different purposes, access, and retention.
2. **Immutability at three levels:** no `UPDATE`/`DELETE` grant for the application role, plus a blocking trigger, plus a hash chain with a gapless tenant-local sequence number and a verification endpoint. External anchoring of the daily chain end value is optional and is named as such — no pretence of tamper-proofing against an attacker with full database access.
3. **Metadata instead of content:** the trail contains diffs of only the changed fields, and their values are masked according to the classification from the data catalogue (`OPEN` in clear text, `SENSITIVE` only as a change marker with a hash, `SECRET` not at all). That decouples the trail from the deletion obligation on content and lets it have a long retention period.
4. **Denormalised actor and target labels**, so that entries stay readable after the referenced entities are deleted.
5. **Creation in the application layer**, in the same transaction as the business change; denied access is recorded centrally in the `AuthorizationService`. Adapters do not audit.
6. **A declaration obligation with a gate:** every use case with security or data protection relevance carries an audit declaration in the use case registry; if it is missing, the build fails (SG-13). That way the trail applies automatically to REST, MCP, and automation alike.
7. **A dedicated `AUDITOR` role** with read access to the audit and the configuration, but no access to content.

## Options considered

| Option | Assessment |
|---|---|
| **Chosen: a dedicated append-only table with a hash chain** | Meets the evidence and deletion requirements simultaneously; extra effort in the schema and the tests. |
| Derive the audit from domain events | No failed attempts, no denied access, no read access; short retention; events are an integration contract, not an instrument of evidence. Rejected. |
| Extend `activity_entry` | It is deleted with the item — useless precisely when evidence is needed. Rejected. |
| Structured operational logs only | Ephemeral, cross-tenant, without access control for tenant administrators, and not queryable. Rejected. |
| An external audit system as a mandatory dependency | Contradicts C-05 (only PostgreSQL is mandatory) and the self-hosting goal. It remains possible as an optional export (A-3). |
| Blockchain / an external transparency log as the standard | Disproportionate operating effort for the benefit; provided for as optional anchoring. |

## Consequences

**Positive**

* Evidence is available even for events without a state change; tampering inside the database becomes detectable.
* The trail survives deletion of accounts and items without undermining the deletion obligation on content.
* Completeness is enforced rather than hoped for: new features without an audit declaration fail the build, and channel parity applies automatically.
* Auditors get access through a dedicated role instead of needing administrator rights.

**Negative / countermeasures**

* *An additional write per operation.* → One insert without foreign key checks, monthly partitioning, only two indices; the impact is measured in the load test.
* *The hash chain creates a serialisation point per tenant.* → A chain per tenant rather than globally; batch chaining with a time window remains the growth path if needed.
* *Masked diffs are less informative than full text.* → A deliberate compromise; anyone needing more uses the business history on the item, which is subject to deletion.
* *Duplicate recording (audit and activity) can diverge.* → Test AT-6 checks channel parity, AT-5 checks atomicity.
* *Long retention increases storage requirements.* → Partitioning and a documented, configurable period; changing the period is itself audited.
