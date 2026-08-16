# ADR-0020: Retention rules as configurable data, with a grace period

* **Status:** accepted
* **Date:** 2026-08-14
* **Concerns:** domain, data protection, operations
* **Related:** [ADR-0018](./ADR-0018-privacy-by-design.md), [ADR-0021](./ADR-0021-offline-sync.md), [data-retention.md](../architecture/data-retention.md)

## Context

Retention periods should be configurable for the main features — "completed tasks for at most a
year, then delete automatically", for example. At the same time GDPR Art. 5(1)(e) requires storage
limitation, and the trash (F-09) and the permanent archive (F-10) already carry their own, partly
contradictory commitments.

Automatic deletion is the only operation in the system that destroys people's work irrecoverably.
It also collides with legal hold, ongoing data subject requests, backups, and devices that have been
offline for a long time.

## Decision

1. **Periods are data, not code:** `retention_policy` per tenant, hub, or collection, with a `data_kind`, an optional CEL condition, a period, an action, and a follow-up stage. A new data kind is added to the catalogue and is immediately configurable.
2. **Multi-stage chains** rather than all-or-nothing: completed → archive after *n* days → delete after *m* more days.
3. **Two-phase execution with a grace period:** mark and warn, and only then execute. Objects in their grace period are visible through the API (`retention.effective_at`) and can be taken out.
4. **Safeguards take precedence**, in this order: legal hold, restriction of processing (Art. 18), lower bounds per data kind, upper bounds with a justification requirement, the minimum tombstone period for offline clients, and referential safeguards.
5. **A safety switch against mass deletion:** a newly activated rule that would affect more than 5% of the holdings on its first run starts in notify-only mode.
6. **Completeness is mandatory:** a hard delete serves every storage location recorded in the data catalogue (row, media, search index, vectors, counters); an orphan test runs after every run.
7. **The audit is exempt** — there, pseudonymisation replaces deletion ([audit.md](../architecture/audit.md) §6).

## Options considered

| Option | Assessment |
|---|---|
| **Chosen: rules as data, with a grace period and precedence rules** | Meets configurability and data protection; more effort because of the two phases and the preview. |
| Fixed periods in the code | Simple, but neither tenant-specific nor adaptable to different legal situations. |
| Single-phase deletion without a grace period | Less code, but the first misconfigured rule destroys data without warning — indefensible. |
| Archive only, never delete | Violates storage limitation and does not solve the cost problem. |
| Leaving deletion to the operator through SQL | Bypasses permissions, the audit, offline consistency, and media cleanup; it creates orphans. |

## Consequences

**Positive**

* The requested case ("completed to-dos for at most a year") is configuration, not development.
* Data protection storage limitation and business tidying use one mechanism rather than two.
* Users are not surprised: a grace period, visibility, advance warning, and a way to object.

**Negative / countermeasures**

* *Two phases double the states and the tests.* → The fixed test catalogue RE-1…RE-9, with time boundaries across time zones and DST as golden tests.
* *CEL conditions in deletion rules are powerful and therefore dangerous.* → A mandatory preview before activation, the 5% switch, and an audit entry with the scope of each run.
* *The minimum tombstone period delays real deletion.* → The period is configurable and set against the maximum offline window; documented in the data catalogue.
* *Large deletion runs load the database.* → Batches, throttling, a dedicated pool, metrics, and a backlog display.
