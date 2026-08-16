# ADR-0008 — Jobs, scheduling, and leader election in PostgreSQL

**Status:** accepted · **Date:** 2026-08-14

## Context
What is needed: reminders at an exact time, materialisation of recurring tasks (RRULE, correct
across time zones), the 30-day trash retention, webhook retries, media cleanup, email sending, AI
jobs. Several replicas must not produce double execution, and self-hosted operation must not require
an additional component.

## Decision
A job queue in PostgreSQL (`job` with `run_at`, `state`, `attempts`, `dedupe_key`), picked up
through `SELECT … FOR UPDATE SKIP LOCKED`. The roles: `worker` (parallel, any number) and
`scheduler` (exactly one active process, secured through `pg_try_advisory_lock`; the work itself is
distributed as jobs). Recurrence follows **RFC 5545 RRULE** through a proven library (`rrule-go`)
with a stored IANA time zone; occurrences are materialised for a rolling window (90 days by
default). Jobs are idempotent, have a timeout and exponential backoff, and land in the dead letter
queue after n attempts, visible in the API.

## Options
1. **A PostgreSQL queue plus an advisory lock leader (chosen).**
2. Redis/Asynq or RabbitMQ — better throughput characteristics, but another mandatory component.
3. Kubernetes CronJobs — unavailable in Docker operation, and no fine-grained timing.
4. An in-process ticker without coordination — double execution with several replicas.
5. Our own RRULE implementation — DST and edge cases are a well-known source of bugs.

## Consequences
**Positive:** no additional infrastructure; jobs can be created transactionally alongside business
data (the outbox pattern); visibility and repeatability through SQL; identical behaviour in Docker
and Kubernetes.
**Negative:** polling overhead and vacuum pressure; a lower throughput ceiling than a dedicated
broker; a leader change can cause a delay of a few seconds.
**Countermeasures:** the `queue` port allows a broker adapter later; partitioning and retention of
the job table; the queue depth and occurrence lag metrics; golden tests for DST and leap year cases.
