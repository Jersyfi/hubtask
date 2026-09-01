<!-- SPDX-License-Identifier: BUSL-1.1 -->
# RB-A01 — The error budget is burning fast enough to be an outage

**Alert:** `HubtaskErrorBudgetBurnAcute` · **Severity:** page · **Catalogue:** A-01 · **SLO:** SLO-1

## The arithmetic

SLO-1 promises 99.9% of requests without a `5xx` over 30 rolling days, so the **error budget** is
0.1% of a month's requests. The **burn rate** is how many times faster than "exactly on budget"
the errors are arriving: a ratio of 0.1% failing requests is burn 1 (the budget lasts exactly 30
days), 1.4% is burn 14 — the budget is gone in 30/14 ≈ **2.1 days**.

The alert is **multiwindow**: it requires the burn over the last *hour* AND over the last *five
minutes*. The hour says the budget is genuinely being spent — a single bad minute cannot trip it.
The five minutes say it is *still* happening — without that, the hour window would keep paging
for 55 minutes after the outage ended, because it remembers it. Both windows read the recorded
series `hubtask:slo1_error_ratio:rateXX` from the `hubtask-slo` group in the same file.

A-02 is the same construction one notch down: burn 6 over six hours (budget gone in five days),
confirmed by the last thirty minutes. Together the pair covers the outage you notice and the one
you would not.

## The symptom

More than 1.4% of all requests are answered `5xx`, for at least an hour, and it has not stopped.
Users are seeing errors right now.

## Immediate action

```bash
# Which routes are failing, and with what?
curl -s localhost:9090/metrics | grep 'hubtask_http_requests_total{.*5xx'

# Is a dependency down? The health report says which.
curl -s localhost:9090/meta/health | jq '.checks'

# Did a rollout just happen? A version spread with errors is a bad deploy.
curl -s localhost:9090/metrics | grep hubtask_build_info
```

* **One route, all instances** → a defect in that handler; check `hubtask_panics_recovered_total`
  and the logs for its error code, roll back if it arrived with the last deploy.
* **All routes, one dependency down** (`hubtask_dependency_up == 0`) → fix the dependency; the
  errors are the middleware failing closed, which is correct.
* **All routes, pool saturated** (`hubtask_db_pool_connections`, A-11's view) → something is
  holding connections; see RB-A11.

## Diagnostic queries

```promql
hubtask:slo1_error_ratio:rate1h
sum by (route) (rate(hubtask_http_requests_total{status_class="5xx"}[10m]))
sum by (use_case, result) (rate(hubtask_usecase_total{result=~"internal|unavailable"}[10m]))
```

## Escalation

This is the page that means "stop what you are doing". If the cause is a deploy, roll back first
and diagnose second — the budget does not come back.

## Follow-up

Every A-01 firing deserves a written incident note: what burned, for how long, what it cost the
budget (the ratio × the duration), and what would have caught it earlier. Once 50% of the budget
is gone, stability work takes precedence over features — that rule is §2's, not this file's.
