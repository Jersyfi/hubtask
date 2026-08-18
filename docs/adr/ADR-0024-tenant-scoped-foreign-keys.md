# ADR-0024 — Tenant-scoped foreign keys

**Status:** accepted · **Date:** 2026-08-18

## Context

[ADR-0010](./ADR-0010-multi-tenancy.md) makes row level security the boundary between tenants and
states what that buys: *"isolation even in the presence of application bugs"*.
[multi-tenancy.md](../architecture/multi-tenancy.md) §2.2 puts it as the fourth and last line of
defence — *"RLS is the last, unbypassable boundary"*.

Implementing B-03 showed that the promise does not hold for **references**. PostgreSQL checks
referential integrity in internal triggers that run as the table owner and are not subject to row
level security. A foreign key therefore sees rows the querying tenant cannot, and this is not a
configuration mistake — it is how the feature is specified.

The schema has 36 single-column foreign keys between tenant-scoped tables and no composite ones,
across 55 tables carrying `tenant_id`. Four consequences were measured against PostgreSQL 16.15
with the application role (`hubtask_app`, no `BYPASSRLS`, not an owner):

1. **A cross-tenant reference can be written.** An insert in tenant B naming tenant A's collection
   succeeds. The row belongs to tenant B — `tenant_id` comes from `current_tenant_id()`, so that
   part of the boundary holds — but its reference points outside the tenant.
2. **The reference dangles from its own tenant's view.** Tenant B then holds an item whose
   collection tenant B cannot read: through the API, an item in a collection that does not exist.
3. **The foreign key is an existence oracle.** Referencing a UUID that exists nowhere fails with
   `23503`; referencing another tenant's real row succeeds. The difference in outcome answers
   "does this identifier exist in this installation" across the tenant boundary — precisely what
   multi-tenancy.md §2 forbids when it says that anything other than one answer *"would confirm the
   existence of another tenant's data"*.
4. **One tenant's ordinary action destroys another tenant's data.** `ON DELETE CASCADE` is not
   subject to row level security either. Tenant A deleting its own container deleted tenant B's
   row. This is the serious one: not a leak, a loss.

None of the four is reachable through the application today. Every use case resolves a reference
through a repository first, under row level security, and refuses what it cannot see — B-03 answers
`items.collection_not_found`. The gap is what happens when that step is forgotten, which is exactly
the case ADR-0010 claims to have covered.

## Decision

Every foreign key between two tenant-scoped tables becomes **composite**: the reference carries
`tenant_id` and points at `(tenant_id, id)` of the referenced table.

```sql
ALTER TABLE work_item ADD CONSTRAINT work_item_collection_id_fkey
  FOREIGN KEY (tenant_id, collection_id) REFERENCES container (tenant_id, id)
  ON DELETE CASCADE;
```

The rule is enforced structurally rather than remembered: a gate walks `pg_constraint` and fails
the build on any single-column foreign key between two tables that carry `tenant_id`, with a
documented exception list — the same shape as the existing gate that proves row level security is
active on every tenant table.

Three details decide whether this works, and each was measured rather than assumed:

* **The referenced table needs `UNIQUE (tenant_id, id)`.** Thirteen tables are referenced and need
  one. The primary key stays on `id` alone: identifiers are UUIDv7 and globally unique, queries
  look rows up by identifier, and moving the primary key to a composite would leave `WHERE id = $1`
  without a unique index.
* **`MATCH SIMPLE` is required, and it is the default.** With it, a row whose reference is `NULL`
  is not checked at all — which is what a nullable reference needs. `MATCH FULL` would demand that
  every column of the key be null together, and `tenant_id` is never null, so every optional
  reference would break.
* **`ON DELETE SET NULL` needs a column list.** Five of the 36 use it, and the naive composite form
  would null `tenant_id` as well, which is `NOT NULL`. PostgreSQL 15 added
  `ON DELETE SET NULL (column)` for this. The trap is that the naive form is **accepted when it is
  declared** and fails only when it fires, with a not-null violation at delete time — so a review
  that reads the migration is not enough, and the gate has to see the delete rule too.

## Options

1. **Composite foreign keys, enforced by a gate (chosen).** The database refuses a cross-tenant
   reference, so all four measured consequences disappear — including the existence oracle, because
   a foreign identifier and a nonexistent one then produce the identical error.
2. **Leave it, and rely on the application layer.** Nothing to build, nothing to migrate. But
   ADR-0010's claim would have to be weakened to "isolation as long as no use case forgets to
   resolve a reference", and every future use case inherits a rule it has to remember. This project
   systematically replaces such rules with structure — the use case registry, the unit of work, the
   gates — and this is the same kind of rule.
3. **Validate with triggers instead.** A trigger per reference costs a query per write, and one
   that reads the parent table runs into the very ambiguity about row level security that this ADR
   is about. It buys nothing a foreign key does not.
4. **Composite primary keys `(tenant_id, id)` throughout.** The same guarantee, and it changes every
   primary key, every join, every generated query, and the identifier type in the domain. Far more
   churn for the same result.

## Consequences

**Positive:** a cross-tenant reference becomes impossible to write rather than merely refused
before it is attempted; cascades stay inside a tenant, so no tenant can delete another's rows; the
existence oracle closes; ADR-0010's claim becomes true as written. The new index is
`(tenant_id, id)` — tenant first, which is the index layout ADR-0010's own countermeasures already
ask for, so it is not dead weight.

**Negative:** thirteen additional unique indexes cost storage and write time on the referenced
tables; 36 constraints have to be swapped; every future table joins the rule, and the gate is what
tells its author so. `ON DELETE SET NULL (column)` requires PostgreSQL 15, which the support matrix
already exceeds (16 and 17) — but it becomes a hard floor rather than a preference, and belongs in
[support-matrix.md](../architecture/support-matrix.md).

**Migration:** forward-only and safe for a rolling update (ADR-0003, rule 12). The unique indexes
are created `CONCURRENTLY`; each constraint is added `NOT VALID` and validated in a second step, so
neither blocks writes. Old application code keeps working throughout, because it already only
writes references inside its own tenant — that is what makes the constraint addable at all. An
installation whose data already violates it fails at `VALIDATE CONSTRAINT` rather than at deploy
time, with the offending rows nameable by a query.

**Not covered:** references to tables with no tenant (`job`, and the system rows of
`item_capability_profile`, whose `tenant_id` is nullable by design). They stay single-column and
are the gate's documented exceptions, for the reason the row level security gate already exempts
them.
