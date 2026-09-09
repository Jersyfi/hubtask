# ADR-0050 — pgvector is a capability, not a requirement

**Status:** proposed · **Date:** 2026-09-09

## Context

`db/migrations/0001_init.sql` line 43 has carried one commented-out line since the first day of the
project:

```sql
-- CREATE EXTENSION IF NOT EXISTS vector;   -- optional, semantic search
```

`0.7.0` is the milestone that has to decide what to do with it
([`milestone-0.7.0.md`](../backlog/milestone-0.7.0.md), J-09, J-10), and the comment is more
load-bearing than it looks. **Uncommenting it would break every existing installation.** Neither
`postgres:16-alpine`, which the reference Compose stack runs, nor
`ghcr.io/cloudnative-pg/postgresql:17.6`, which the chart defaults to, ships the extension —
`CREATE EXTENSION` on an image that does not have the files fails, and a failed migration is a
stack that will not start (`cmd/migrate` refuses to serve behind a migration that did not apply).

So the question is not "shall we use pgvector". It is: **what does an installation that has not got
it do**, and **what do the images this project publishes carry**. Those two are one decision,
because the second is what makes the first rare.

Three constraints shape it:

* **`support-matrix.md` §1**: a status is "a CI job runs the software on it". A semantic search
  nothing exercises may not be called supported.
* **Rule 12**: migrations are forward-only and safe for rolling updates. A migration that fails on
  some installations is not a migration.
* **`ai-first.md` §3 and the degradation table**: "an external search index (optional) → fallback to
  PostgreSQL full-text search" is already the documented shape for a search that is not there.

## Decision

**1. The extension is detected, never demanded, and creating it is the operator's act.** Migration
0075 asks three questions in order, the way migration 0019 asks `pg_ts_config` before it names a
text search configuration: is the extension already installed — by an operator, by a managed
service's console, by an earlier run — then use it; is it *available* — then try to create it, and
treat a refusal as absence, because `vector` is not a trusted extension and a migrator role on a
managed PostgreSQL is not a superuser (the wall H-10 met under CloudNativePG); otherwise do nothing
at all. In every case the migration succeeds, and where the store is absent search is lexical and
nothing else changes.

**2. The embeddings live in their own table, not in a column of `work_item`.** A conditional
*column* would give one table two shapes and every query over it two meanings. A conditional
*table* is one object that is either there or not, which a capability check can answer with a
single question — and it keeps the vector out of the row every write of an entry touches.

**3. `/meta/capabilities` publishes whether semantic search exists**, beside `text_languages` and
for the same reason ADR-0034 gives: a client's controls come from data rather than from a constant
compiled into it, and an installation that cannot do something says so instead of failing when
asked.

**4. The reference images carry the extension; the chart's default does not.**

| Where | Image | Semantic search |
|---|---|---|
| `deploy/docker/compose.yaml`, `compose.dev.yaml` | `pgvector/pgvector:pg16` | yes |
| Every gate that starts a PostgreSQL | the same | yes — which is what lets the support matrix say it |
| `k8s/values.yaml` (`database.imageName`) | `ghcr.io/cloudnative-pg/postgresql:17.6`, unchanged | no, until an operator sets an image that has it |
| A managed PostgreSQL | whatever the provider offers | most offer it; the installation finds out at migration time and is told in `/meta/capabilities` |

The split is deliberate. A self-hoster following the Compose reference gets the whole product. An
operator running the chart against a database they or their platform owns is *not* handed a new
image requirement by an upgrade — `ADR-0046` put the database in somebody else's hands on purpose,
and a minor release that demanded a different image would be exactly the kind of surprise that ADR
exists to prevent. What they get instead is a documented value and a manifest that tells them where
they stand.

**5. The store's statements are hand-written, in the one place hand-written SQL already lives.**
sqlc reads `db/migrations`, and a table created inside a `DO` block is invisible to it — which is
not a limitation of sqlc but a direct consequence of decision 1: a conditional table cannot be
generated against. So the statements the store needs are constants with bound parameters in
`infrastructure/postgres`, beside the query compiler [ADR-0026](./ADR-0026-query-dsl-sql-construction.md)
already excepted from rule 9. What they keep is the half of that rule that matters: **no byte of a
request becomes SQL text** — every value is bound, and the only thing assembled is the search's
ranking expression, which is the exception ADR-0026 bounds.

**6. The support matrix says `supported` for semantic search and names the job**, because after
decision 4 a job does run it. `PostgreSQL without pgvector` is a supported configuration too, and
what it is supported *for* is lexical search — which is what the row will say.

## Options

1. **Detected, with the reference images carrying it (chosen).**
2. **Required: uncomment the line and publish a new image.** Simple, and it breaks every
   installation that upgrades without changing its database image — including every managed one
   whose provider does not offer the extension. Rejected on rule 12 alone.
3. **Detected, and no image changes.** Then no gate ever runs the semantic half, the support matrix
   may not claim it (§1), and the feature ships untested. Rejected: this is how a feature comes to
   exist only in the documentation.
4. **A separate search service (Elasticsearch, Meilisearch, Qdrant).** The degradation table already
   lists "external search index (optional)" as a thing that may exist, so this is not forbidden —
   but it is a second piece of infrastructure a self-hoster has to run, which
   [ADR-0003](./ADR-0003-postgresql-as-single-datastore.md) decided against for exactly this class of feature. It
   also does not remove the question: an installation without the service still needs an answer,
   and that answer is this one.

## Consequences

**Positive:** no installation breaks on upgrade; a self-hoster gets semantic search by following
the reference stack; every gate exercises the semantic path, so the support matrix can say
`supported` honestly; an installation that cannot do it says so through the manifest rather than by
failing; and the fallback is not a new mechanism — it is the lexical search that already exists.

**Negative:** two code paths through search, and one of them is exercised by a test that
deliberately has no extension rather than by the default gate. The reference Compose image changes,
which is a line in an upgrade note. And an operator on the chart who wants semantic search has to
choose an image, which is one more thing to know.

**Countermeasures:** the capability is read once and answered from one place, so the two paths meet
in one function rather than at every call site; a migration test runs against a PostgreSQL *without*
the extension and asserts that it applies and that the store is absent; and `deployment.md`
documents the chart value beside the image it defaults to.

**One thing this ADR got wrong on its first attempt**, recorded because the reasoning is worth
keeping: the index was to be built `CONCURRENTLY` in a migration of its own, on the discipline
migrations 0019 and 0020 established for the search document. That discipline is about an index over
a *populated* table, where `ACCESS EXCLUSIVE` blocks the previous version's pods for as long as the
build takes. Here the table is created empty in the same migration, so the lock is over something no
pod has ever read — and `CREATE INDEX CONCURRENTLY` cannot run inside a `DO` block at all, which is
what a conditional migration has to be. The index is built in place, plainly.
