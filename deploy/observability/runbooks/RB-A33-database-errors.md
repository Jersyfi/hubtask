<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A33 — Database statements are failing

**Alert:** `HubtaskDatabaseErrors` · **Severity:** ticket · **Catalogue:** A-33

## The symptom

For a quarter of an hour more than one statement every ten seconds failed with the kind in the
label `kind` (`hubtask_db_errors_total`): `timeout` (a statement or lock wait ran out), `connection`
(the connection was lost or refused) or `other` (an error the adapter does not classify).
Serialisation failures are retried and constraint violations are answers; neither counts.

## Immediate action

```promql
sum by (pool, kind) (rate(hubtask_db_errors_total[10m]))
```

* **`connection`** → the database restarts, fails over, or refuses connections at its limit
  (`max_connections`, a pooler's limit); A-11 shows the pools' saturation.
* **`timeout`** → slow statements or lock waits: `hubtask_db_slow_queries_total` names the query;
  `pg_stat_activity` shows what waits on what; an old transaction (A-25) is a common cause.
* **`other`** → the server log carries the codes; a disk that is full or read-only shows here first.

## Afterwards

Requests that failed answered `503` and are retried by their clients; jobs are retried by the
queue. Nothing needs replaying once the cause is gone.
