<!-- SPDX-License-Identifier: BUSL-1.1 -->
# RB-A15 — Authentication is being refused, or a token was used twice

**Alerts:** `HubtaskAuthFailureRate` (ticket) · `HubtaskRefreshTokenReused` (page) · **Catalogue:** A-15

## The symptom

Two alerts share this catalogue row because the row has two halves:

* **The rate** (ticket): more than 5% of attempts on the auth routes are refused. Either a
  credential-stuffing run, or a client somewhere is broken and retrying — a rotated secret
  whose consumer was not updated produces exactly this.
* **The reuse** (page): a refresh token that had already been rotated away was presented again.
  Two holders of one credential. H-01's rotation already revoked the whole session family the
  moment it happened — the machine's response is done; the page is for the human question of
  *how* there were two holders.

## Immediate action

```bash
# Which reasons dominate? The label set is closed and carries no subject.
curl -s localhost:9090/metrics | grep hubtask_auth_failures_total
```

* **`wrong_credential` dominating, spread over time** → stuffing or a scripted client. The rate
  limiter is already pricing it (`hubtask_rate_limited_total{scope="ip"}`); check whether the
  source concentration justifies a block at the ingress.
* **`refresh_refused` dominating, starting at a deploy or rotation** → a client with a stale
  secret; find which integration updated last.
* **`refresh_reused` (the page)** → read the audit log for the affected family - the audit
  entry names the session without naming the token. If the legitimate user is active and
  unaware, treat as credential theft: force re-authentication, advise a password change, and
  open a security incident. A reuse from the same IP class seconds apart is usually a retrying
  client that lost the rotation answer — the audit trail's timing tells the two apart.

## Diagnostic queries

```promql
sum by (reason) (rate(hubtask_auth_failures_total[15m]))
sum(rate(hubtask_http_requests_total{route=~"/auth/sessions.*"}[15m]))
increase(hubtask_auth_failures_total{reason="refresh_reused"}[24h])
```

## Escalation

The reuse page escalates to a security incident the moment more than one account is involved —
that pattern is infrastructure compromise, not one stolen laptop.

## Follow-up

Repeated false pages from retrying clients that lost a rotation answer are a known cost of
strict reuse detection; before relaxing anything, measure how often it is genuinely benign. The
strictness is the feature.
