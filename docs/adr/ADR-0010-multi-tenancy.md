# ADR-0010 — Multi-tenancy through a shared schema and row level security

**Status:** accepted · **Date:** 2026-08-14

## Context
The application is meant to be offered by service providers "at scale to end users" (Atlassian and
Trello as the model) and at the same time to run as a single installation. Data leaks between
tenants would be existential. Migrations must stay manageable with thousands of tenants.

## Decision
Shared database, shared schema: `tenant_id uuid NOT NULL` in every business table, and
**PostgreSQL row level security** with `FORCE ROW LEVEL SECURITY` as the enforced boundary. The
application role holds neither `BYPASSRLS` nor the tables; migrations run with a separate role.
The tenant context is set in exactly one place through `SET LOCAL app.tenant_id` per transaction
(pool-safe). Two modes: `single` (self-hosting, one implicit tenant) and `multi`.
The growth path for data residency and enterprise isolation: shard routing through a control plane
mapping `tenant → shard`, with no change to the data model.

## Options
1. **Shared schema + RLS (chosen).**
2. A schema per tenant — stronger isolation, but migrations × n, and connection and catalogue pressure with thousands of tenants.
3. A database per tenant — the strongest isolation, the highest operating cost, and overhead for self-hosting.
4. Application-side filtering only, without RLS — one forgotten `WHERE` is enough for a data leak.

## Consequences
**Positive:** one migration for everyone; minimal resources per tenant; an identical code path for
self-hosting and the platform; isolation even in the presence of application bugs; the groundwork
for sharding is in place.
**Negative:** the "noisy neighbour" problem must be solved through quotas; RLS predicates must be
accounted for in every index; no tenant can trivially be restored on its own (a restore concept is
needed); connection pooling requires care (`SET LOCAL`, never `SET`).
**Countermeasures:** indices begin with `tenant_id`; quotas and a weighted queue; cross-tenant
negative tests for every repository in CI; a documented single-tenant export and restore path.
