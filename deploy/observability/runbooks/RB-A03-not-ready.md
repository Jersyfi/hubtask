<!-- SPDX-License-Identifier: BUSL-1.1 -->
# RB-A03 — Hubtask is not ready to serve

**Alert:** `HubtaskNotReady` · **Severity:** page · **Catalogue:** A-03

## The symptom

Either the scrape fails (`up == 0`: the process is gone, the port is closed, the network is cut) or
the process is running and reports its mandatory dependency down (`hubtask_dependency_up{dependency="postgres"} == 0`).
Users see `503` with a `Retry-After`. Nothing is lost — writes are refused, not accepted and dropped.

## Immediate action

```bash
docker compose ps                       # or: kubectl get pods -l app.kubernetes.io/instance=hubtask
curl -s localhost:9090/readyz           # the process's own answer, if it answers at all
curl -s localhost:9090/meta/health | jq # which dependency, and since when
```

The distinction that decides everything: **does the process answer at all?**

* **No answer** → the process is down. `docker compose logs app --tail 100` or
  `kubectl logs deploy/hubtask-api --previous`. A crash loop with a configuration error exits at
  startup by design (fail closed) — the log line names the variable.
* **Answers, but `readyz` red** → the process is fine and PostgreSQL is not. Check the database
  container/pod, disk space (`df -h`), and connection count
  (`SELECT count(*) FROM pg_stat_activity`). Hubtask reconnects by itself with backoff; it does
  **not** need a restart once the database returns (ADR-0016).

## Diagnostic queries

```promql
up{job="hubtask"}                                  # is anything being scraped at all
hubtask_dependency_up                              # per dependency, 1 up / 0 down
hubtask_build_info                                 # which version is actually running
rate(hubtask_http_requests_total{status_class="5xx"}[5m])
```

## Escalation

None — self-hosting has one operator. If the database is unrecoverable, this becomes a restore:
see [backup-restore.md](../../../docs/architecture/backup-restore.md).

## Follow-up

A readiness outage that was **not** a database outage means a dependency probe is wrong or a
timeout is too tight. Record what it was; if the process exited on a configuration error, the
error message should have named the variable — if it did not, that is a defect worth an issue.
