<!-- SPDX-License-Identifier: BUSL-1.1 -->
# RB-A04 — A panic was recovered

**Alert:** `HubtaskPanicRecovered` · **Severity:** page · **Catalogue:** A-04

## The symptom

`hubtask_panics_recovered_total` rose. The target value is permanently zero (ADR-0016): the process
survived — `SafeGo` and the request middleware catch panics so one bad request cannot take the
installation down — but **a panic is always a program defect**, never load and never a user's fault.

The user whose request it was got a `500` with a request ID. A panicking job was retried and may
have gone to the dead letter (see [RB-A07](./RB-A07-dead-letter.md)).

## Immediate action

Nothing operational. Do **not** restart: the process is not degraded, and a restart discards the
evidence. Collect it instead:

```bash
docker compose logs app --since 30m | grep -A 40 "panic recovered"
```

The log line carries the component, the stack trace, and the request or trace ID. The panic value
itself is logged through the redacting logger — assume it may contain user content and treat the
log excerpt accordingly when attaching it to an issue.

## Diagnostic queries

```promql
increase(hubtask_panics_recovered_total[1h])              # how often, and
sum by (component) (hubtask_panics_recovered_total)       # where
```

`component` is the location: `rest.request`, `worker.job.<kind>`, `server.api`.

## Escalation

This is a bug report. Open an issue with the stack trace, the component, the version
(`hubtask_build_info`), and what the request or job was doing. It is the one alert whose
resolution is always a code change.

## Follow-up

A recovered panic that produced no test is a panic that will come back. The fix belongs with a
regression test at the layer where it happened.
