<!-- SPDX-License-Identifier: BUSL-1.1 -->
# RB-A10 — A circuit breaker has been open for ten minutes

**Alert:** `HubtaskCircuitBreakerOpen` · **Severity:** ticket · **Catalogue:** A-10

## The symptom

The breaker for one guarded dependency is at state 2: calls are refused without being attempted,
because enough of them failed in a row. Ten minutes open is past a blip — the dependency is
persistently unavailable, and whatever needs it is running degraded (`hubtask_degraded_mode`
says which feature).

## Immediate action

```bash
# Which dependency, and does the health report agree?
curl -s localhost:9090/metrics | grep -E "hubtask_circuit_breaker_state|hubtask_dependency_up"
curl -s localhost:9090/meta/health | jq '.checks'
```

* **`smtp`** → reminders and notifications queue rather than send; nothing is lost, but A-08
  follows if it stays down. Check the mail provider's status page before anything else.
* **`object_storage`** → media uploads refuse and backups to that target fail; A-12 follows
  within a day. MinIO/S3-compatible stores answering 503 on health while refusing writes is a
  known shape (see the integration gate's history).
* **A webhook or automation target** → an external server; its owner's problem, the breaker is
  doing its job.

The breaker recovers alone: it half-opens on its own schedule and closes on the first success.
Do not restart the process to "reset" it — that only forgets what was learned.

## Diagnostic queries

```promql
hubtask_circuit_breaker_state
changes(hubtask_circuit_breaker_state[1h])
hubtask_degraded_mode
```

## Escalation

If the dependency is this installation's own (the database is never breakered — it is mandatory;
but SMTP and storage are), and it is not a provider outage, escalate to whoever runs it.

## Follow-up

Flapping (many `changes()` per hour) with no outage on the other side means the thresholds are
wrong for that dependency's normal latency — worth an issue rather than a raised threshold in
production.
