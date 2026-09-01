<!-- SPDX-License-Identifier: BUSL-1.1 -->
# RB-A09 — Webhook deliveries are failing

**Alert:** `HubtaskWebhookDeliveryFailing` · **Severity:** ticket · **Catalogue:** A-09 · **SLO:** SLO-6

## The symptom

More than 20% of webhook attempts over the last half hour did not get through — counting only
what is this system's to deliver: recipient-side `4xx` answers are excluded, exactly as SLO-6
words the objective. A target answering 404 forever is that subscriber's problem and their
subscription will disable itself; this alert is about the rest.

## Immediate action

```bash
# What are the failures? The class label says whether targets answer at all.
curl -s localhost:9090/metrics | grep hubtask_webhook_deliveries_total

# Which subscriptions carry the dead letters?
psql -c "SELECT subscription_id, count(*), max(error_code) FROM webhook_delivery
         WHERE status = 'DEAD_LETTER' GROUP BY subscription_id ORDER BY count(*) DESC LIMIT 10;"
```

* **`status_class="5xx"`** → the targets are up but erroring; usually one big subscriber having
  an outage. Their retries are already scheduled; nothing to do but watch the backlog drain.
* **`status_class="none"`** → attempts never got an answer: DNS, egress, or the guard. Check
  `hubtask_outbound_http_duration_seconds{target_class="webhook"}` and the egress allowlist —
  a `webhooks.target_blocked` error code means the guard refused the address (T-07).
* **Rate limiting (`webhooks.target_rate_limited`)** → one subscriber throttling; the backoff
  ladder already spaces the retries out.

## Diagnostic queries

```promql
sum by (result, status_class) (rate(hubtask_webhook_deliveries_total[30m]))
hubtask_webhook_retry_backlog
histogram_quantile(0.95, sum(rate(hubtask_outbound_http_duration_seconds_bucket{target_class="webhook"}[15m])) by (le))
```

## Escalation

None while the failures are external. If `none` dominates and internal egress broke, that is an
infrastructure incident — the same network path carries backups (A-12 will follow).

## Follow-up

A subscriber that dead-letters regularly should be pointed at their delivery log — the API
exposes it — before their subscription disables itself and they file it as our outage.
