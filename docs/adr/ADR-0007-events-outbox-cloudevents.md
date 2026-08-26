# ADR-0007 — Transactional outbox and CloudEvents

**Status:** accepted · **Date:** 2026-08-14

## Context
Domain changes must reliably reach automation rules, webhook subscribers, the search index, live
updates, and notifications. Events must not be lost when an external recipient fails, and there must
be no event without a corresponding data change (and vice versa). On top of that, events are a
public contract for n8n/Zapier.

## Decision
**Transactional outbox:** domain events are written to the `outbox_event` table in the same
transaction as the data change. A dispatcher (the `worker` role) reads with
`FOR UPDATE SKIP LOCKED` and distributes to consumers: the automation engine, webhook delivery, the
SSE stream, the search index, and optionally NATS JetStream.
**Format:** CloudEvents 1.0 (structured JSON), type
`de.hubtask.<context>.<entity>.<action>.v1`, with `tenantId`, `actor`, `correlationId`,
`causationId`, and `causationDepth`. The delivery guarantee is at-least-once; consumers are
idempotent (`event_id` deduplication). JSON schemas live under `api/events/` and are versioned like
the API.

**`replay`** joined that set with E-06. It marks a change a restore wrote rather than one somebody
made ([backup-restore.md](../architecture/backup-restore.md) §8.4), and it is an extension attribute
rather than a payload field because it is routing: the dispatcher decides what to hand a subscriber
before anything parses `data`. It is present only when true, so a consumer written before restores
existed reads an ordinary event exactly as it always did, and a broker's routing rule can express
"has this attribute" where it cannot express "equals false or missing". A subscriber is given a
replay only if it has asked for one, so the promise does not rest on every consumer remembering it.

## Options
1. **Outbox + CloudEvents (chosen).**
2. Calling external systems directly within the request — latency, partial failures, inconsistent states.
3. Change data capture (Debezium / logical replication) — no application context (actor, intent), and additional infrastructure.
4. A message broker as a requirement (Kafka/NATS) — contradicts ADR-0003.
5. Our own event format — CloudEvents is established and understood by n8n, Zapier, and Knative.

## Consequences
**Positive:** no lost or orphaned events; retry and dead letter handled centrally; automation and
webhooks use the same stream; a standard format eases integrations; the event history helps with
debugging.
**Negative:** additional latency (the polling interval); the outbox table grows and needs cleaning
up; consumers must be idempotent; ordering is guaranteed only per aggregate.
**Countermeasures:** an adaptive polling interval plus `LISTEN/NOTIFY` as a wake-up; partitioning
and retention for `outbox_event`; deduplication as a library function; the "outbox lag" metric with
an alert.
