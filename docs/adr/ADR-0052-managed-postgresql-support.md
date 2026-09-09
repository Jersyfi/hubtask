# ADR-0052 — Managed PostgreSQL is supported, and migration 0002 is corrected to allow it

**Status:** accepted · **Date:** 2026-09-08 · **Accepted:** 2026-09-09

## Context

An installation is meant to have three ways to get a database, and
[deployment.md](../architecture/deployment.md) has always implied all three: PostgreSQL in the
Compose stack, the CloudNativePG `Cluster` this chart can own
([ADR-0046](./ADR-0046-production-on-a-platform-namespace.md)), or **a database the operator brings
and hands over as a connection string**. The chart already supports the third: `database.enabled` is
off by default, and every deployment reads `db-dsn` from a Secret.

`db/migrations/0001_init.sql` anticipates that third case explicitly. Its role block catches
`insufficient_privilege` and says so in a comment: *"A managed PostgreSQL may forbid CREATE ROLE. In
that case the operator creates the two roles beforehand."*

**It does not work, and it never has.** Measured against a PostgreSQL whose owner has `CREATEROLE`
and is not a superuser — which is what AWS, Azure and Google hand out — the migration stops in
`0002_capability_profiles.sql`:

```
ERROR: new row violates row-level security policy for table "item_capability_profile"
```

**Why it stops, and why nobody noticed.** `0001` puts `FORCE ROW LEVEL SECURITY` on every tenant
table. `FORCE` binds the table's **owner** as well; without it, an owner bypasses its own policies.
On `item_capability_profile` the policy deliberately lets everyone read the system defaults and lets
**nobody** write them:

```sql
USING (tenant_id IS NULL OR tenant_id = current_tenant_id())
WITH CHECK (tenant_id = current_tenant_id());
```

`0002` then seeds exactly those `tenant_id IS NULL` rows. So on this one table `FORCE` locks out the
only actor that legitimately writes them: the migrator. It has worked everywhere so far for one
reason only — Compose, CI and the integration environment all run migrations as the PostgreSQL
**superuser**, and a superuser ignores `FORCE`. H-10 hit the same wall from the other side: under
CloudNativePG the owner is not a superuser either, and the chart works around it with
`ALTER ROLE ... BYPASSRLS` in `bootstrap.initdb.postInitSQL`. A managed service offers no equivalent,
because granting `BYPASSRLS` requires a superuser.

**So the position today is:** the documents promise a database an operator brings, the schema makes
it impossible, and the only thing hiding that is a privilege the three exercised environments happen
to have.

## Decision

**1. Managed PostgreSQL is a supported configuration**, with a CI job that proves it, per
`support-matrix.md` §1's definition of the word.

**2. `item_capability_profile` loses `FORCE ROW LEVEL SECURITY`**, and nothing else does. The
policy stays exactly as it is. The effect is precisely: *the owner may seed the system defaults, and
nobody else gains anything.* `hubtask_app` is not the owner, so it stays bound by the policy — it can
read the three defaults, it cannot write them, and it still sees nothing without a tenant context.

**3. That correction is applied in two places, and this is the part that deviates from a rule.**

* `db/migrations/0002_capability_profiles.sql` gains the `NO FORCE` statement before its seed, so a
  **fresh** database can be migrated by a non-superuser owner.
* A new `db/migrations/0077_capability_profile_owner_may_seed.sql` states the same thing for every
  database that applied `0002` before this change, so the two paths converge instead of drifting.

**4. The operator's obligations on a managed instance are written down** rather than implied: create
`hubtask_migrator` and `hubtask_app` beforehand, and let `hubtask_migrator` own the database.

## The rule this deviates from, and why the deviation is bounded

`CLAUDE.md` rule 12: *"Migrations are forward-only and safe for rolling updates (expand/contract).
Never change an existing migration."* Editing `0002` is a deviation, and the reason the rule exists
is drift: a database that applied the old text and one that applies the new text end up different.

Three things bound it, and all three were measured rather than argued (see Evidence below):

* **A database that already applied `0002` never re-reads it.** goose keys on the version, so the
  edit is invisible to every existing installation. What reaches those is migration `0077`, whose
  single statement is the same statement.
* **Both paths end in the identical catalogue state.** A fresh database migrated with the corrected
  `0002` and an old database brought forward by `0077` were compared table by table on row level
  security and force flags. They are the same.
* **The alternative does not exist.** A new migration alone cannot fix a fresh install, because
  `0002` fails before anything later is reached. Nothing in the migrator, the chart or the
  configuration can lift `FORCE` on a table that does not exist yet. The only other honest option is
  to declare managed PostgreSQL unsupported and delete the promise from `0001`'s comment.

## Options

1. **Correct `0002` and add `0077` (chosen).** One statement, in two places, with the identical
   effect. Costs a documented exception to rule 12.
2. **Declare managed PostgreSQL unsupported.** Honest, cheap, and removes a capability the documents
   have implied since `0001`. It also leaves the schema with a table whose `FORCE` locks out its own
   writer, which would remain true and unexplained.
3. **Move the system defaults out of the database** into code, so nothing has to seed them.
   A larger change to the domain model, which `domain-model.md` puts in the database on purpose, and
   it would not make the `FORCE` on that table any less self-contradictory.
4. **Have the migrator lift `FORCE` around the seed at run time**, by applying `0001`, altering the
   table, then continuing. It works, and it puts knowledge of one migration's contents into the
   migrator program, where the next such seed would silently not be covered.

## Consequences

* `item_capability_profile` joins the documented row-level-security exception list — in
  `test/integration`, and in the restore drill's own copy, each with the reason.
* `db/schema.sql` mirrors the change, as it mirrors every other migration.
* A new integration test migrates as a non-superuser owner and asserts the boundary afterwards. It
  is the thing that keeps this true: the defect survived this long precisely because no environment
  exercised it.
* `support-matrix.md` gains a row for managed PostgreSQL, and the row has a job behind it.
* The tenant boundary is unchanged for the application role. That is the claim this decision turns
  on, so it is asserted in the new test rather than left as a sentence here.

## Evidence

Measured before this ADR was written, against PostgreSQL 17 in a container, with an owner that has
`CREATEROLE` and is not a superuser:

| Question | Result |
|---|---|
| Today's migrations on such an instance | Stop at `0002`, row level security violation |
| With the correction, fresh | All migrations apply |
| With the correction, an existing database | Only `0077` applies; `0002` is not re-run; the seeded rows are untouched |
| Fresh versus brought-forward | Identical row-level-security state on every table |
| `hubtask_app` afterwards | Not superuser, no `BYPASSRLS`, cannot write the system defaults, reads them, sees nothing without a tenant |
| Tenant isolation afterwards | Two workspaces created through the application role; neither sees the other; a write across the boundary has no effect |
| Tables newly without `FORCE` | Exactly one: `item_capability_profile` |
| The Compose path (superuser) | Unchanged |
