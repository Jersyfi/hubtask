# ADR-0003 — PostgreSQL as the only mandatory datastore

**Status:** accepted · **Date:** 2026-08-14

## Context
What is needed: relational storage, tree queries, flexible fields, full-text search, a job queue,
an event outbox, pub/sub, tenant isolation, and optionally vector search. Private individuals should
be able to run the application with minimal effort; every additional mandatory component (Redis,
Kafka, MinIO, Elasticsearch) lowers adoption and raises operating costs.

## Decision
**PostgreSQL 16+ is the only mandatory dependency.** What is used:
`jsonb` + GIN for custom fields, generated `tsvector` columns for full text, `pg_trgm` for CJK and
fuzzy matching, `SELECT … FOR UPDATE SKIP LOCKED` for the job queue and outbox dispatch, advisory
locks for leader election, row level security for tenant boundaries, and optionally `pgvector` for
semantic search. Access through `pgx/v5`, queries with `sqlc` (type-safe, no ORM magic), migrations
with `goose`.

Every other store is an **optional adapter**: object storage (S3/MinIO, falling back to a local
volume), NATS JetStream (falling back to outbox polling), external search (falling back to
PostgreSQL).

## Options
1. **PostgreSQL only (chosen).**
2. PostgreSQL + Redis + Kafka/NATS + Elasticsearch + MinIO — better peak load handling, untenable for self-hosting.
3. A NoSQL primary store — hierarchy, filtering, transactions, and tenant RLS would all become more work.

## Consequences
**Positive:** one backup, one migration, one monitoring target; a transaction spanning business data
*and* events (the outbox); low operating cost; PostgreSQL knowledge is widespread.
**Negative:** PostgreSQL becomes the bottleneck under extreme load; queue polling creates dead tuple
pressure (vacuum); full-text relevance is weaker than with dedicated search engines.
**Countermeasures:** ports abstract the queue, bus, and search; NATS and vector adapters are
provided for; partitioning of large tables; load tests from milestone 0.6; autovacuum tuning
documented.
