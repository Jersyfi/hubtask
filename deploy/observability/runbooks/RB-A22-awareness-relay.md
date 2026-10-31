<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A22 — Co-editors on different replicas do not see each other

**Alert:** `HubtaskAwarenessRelayFailing` · **Severity:** ticket · **Catalogue:** A-22

## The symptom

A live document's awareness - who else has a page, a whiteboard or an entry open, and where their
cursor is - is handed between replicas as a PostgreSQL notification on `hubtask_awareness`
(offline-sync.md §13). Those notifications have failed for a quarter of an hour. People connected
to the same replica still see each other; people on different replicas do not.

Nothing is lost: a document's words are stored and woken on their own channel, and awareness is
the latest cursor, sent again with the next move. The alert is a ticket, not a page.

## Immediate action

```promql
sum by (outcome) (rate(hubtask_realtime_awareness_relayed_total[10m]))
sum by (pool, kind) (rate(hubtask_db_errors_total[10m]))
```

* **`hubtask_db_errors_total{kind="connection"}` rising with it** → the replicas reach the database
  badly; follow A-11 (pool saturation) or the database's own health first.
* **`kind="timeout"`** → a publication waits longer than two seconds for a connection; the pool is
  saturated by other work. Awareness yields to it by design.
* **Only one replica's series** → that replica's connection to the database is the matter; restart
  it once the database answers it.

## Escalation

Failures on every replica while the database answers every other statement point at the
notification queue (`pg_notification_queue_usage()` near 1): a listener that stopped reading. Treat
that as an incident - it also holds back the change stream's wake-ups.

## Follow-up

The alert resolves ten minutes after the failures stop. `too_large` is not a failure: a state above
about 5.8 KB stays on its own replica and is sent again by the editor.
