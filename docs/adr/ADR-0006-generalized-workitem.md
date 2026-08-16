# ADR-0006 — A generalised WorkItem with capability profiles

**Status:** accepted · **Date:** 2026-08-14

## Context
The requirements name four levels (collection → task → work package → activity) with different
feature sets: a task has every feature, an activity only status, due date, reminder, and assignment.
A generalisation approach, a "central extensible core component", and future-orientation are all
explicitly demanded — further levels and fields are foreseeable.

## Decision
One aggregate root `WorkItem` with a `type` attribute (`TASK`, `WORK_PACKAGE`, `ACTIVITY`,
extensible) and an `ItemCapabilityProfile` per type, which defines which capabilities are active,
which child types are permitted, and how deeply nesting may go. Containers (`HUB`, `COLLECTION`) are
generalised analogously through `Container.type`. Tenant-specific fields come from typed
`CustomFieldDefinition` plus `jsonb`. Violations of a profile produce the explicit error
`capability_not_supported` (no silent ignoring).

## Options
1. **One generalised WorkItem plus capability profiles (chosen).**
2. Four separate aggregates/tables/resources — clear typing, but four times the work for every cross-cutting feature (filtering, automation, permissions, search) and expensive extension.
3. A completely schema-free model (everything `jsonb`) — maximally flexible, but no invariants, poor queryability, no data quality.
4. An EAV model — flexible, but poor performance and complex queries.

## Consequences
**Positive:** one table, one repository, one set of use cases, one API resource `/items`; filtering,
automation, permissions, and search apply automatically at every level; a new level is
configuration; capabilities can be narrowed per tenant.
**Negative:** the risk of a "god object"; invariants are type-dependent and therefore less directly
represented in the type system; the compiler checks less, so tests must check more; fields
irrelevant to a type still exist in the schema (nullable).
**Countermeasures:** capability evaluation as its own domain service; complete matrix tests per
type × capability; the review rule "a new field needs a capability entry"; risk R-01 in arc42 §11
with regular review.
