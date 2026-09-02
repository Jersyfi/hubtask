<!-- SPDX-License-Identifier: BUSL-1.1 -->
# RB-A02 — The error budget is leaking

**Alert:** `HubtaskErrorBudgetBurnCreeping` · **Severity:** page · **Catalogue:** A-02 · **SLO:** SLO-1

## The symptom

More than 0.6% of requests have failed with a `5xx` across the last six hours, and the last
thirty minutes look the same. Burn rate 6: nothing looks like an outage from any single minute,
but the 30-day budget of SLO-1 is gone in **five days** at this pace. The arithmetic and the
multiwindow construction are RB-A01-acute-burn.md's — this is the same alert one notch down,
built to catch what A-01's windows are too short to see.

The classic causes are the ones that do not spike: a retry loop that half-works, one tenant's
integration hammering an endpoint that errors, a dependency that is flapping rather than down, a
deploy that broke one uncommon route.

## Immediate action

```bash
# Which route carries the errors? A creeping burn is usually concentrated.
curl -s localhost:9090/metrics | grep 'hubtask_http_requests_total{.*5xx'

# Is a breaker flapping? Half-open cycles produce exactly this signature.
curl -s localhost:9090/metrics | grep hubtask_circuit_breaker_state
```

* **Concentrated on one route** → read that handler's error code in the logs; this is a defect
  with a small blast radius, which is why it crept instead of paging as A-01.
* **Spread thin and correlated with `hubtask_dependency_up` dips** → a flapping dependency;
  treat the dependency, not the symptom.
* **Started at a deploy** → roll back. Five days of budget is not a reason to debug in
  production.

## Diagnostic queries

```promql
hubtask:slo1_error_ratio:rate6h
sum by (route) (rate(hubtask_http_requests_total{status_class="5xx"}[1h]))
changes(hubtask_circuit_breaker_state[6h])
```

## Escalation

A page, but one with hours rather than minutes in it. If the cause is not found within a working
day, the honest move is to declare the budget spent for this incident and gate feature work per
§2's error budget rule.

## Follow-up

A creeping burn that lived for hours before firing means the dashboards were not being read.
Check whether the SLO dashboard's burn panel would have shown it, and whether A-02's windows are
right for this installation's traffic shape.
