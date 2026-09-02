<!-- SPDX-License-Identifier: BUSL-1.1 -->
# RB-A14 — The process is running with flagged configuration

**Alert:** `HubtaskMisconfigured` · **Severity:** ticket · **Catalogue:** A-14

## The symptom

`hubtask_config_invalid_total` is above zero: a startup check flagged configuration and the
process is running without what it names. The `key` label carries the finding's code, not a
value — the same finding is in `/meta/health`'s warnings with its parameters.

A hard-invalid variable never reaches this alert: it stops the process at startup, before the
metrics exporter exists. What lands here is the tolerated kind — the installation runs, but
something it is supposed to have is missing.

## Immediate action

```bash
# The findings, with their parameters:
curl -s localhost:9090/meta/health | jq '.warnings'
```

The codes and what they cost:

* `config.smtp_missing_with_reminders` — reminders are on and there is no mail server: they
  fire into nothing. The most urgent of the set.
* `config.base_url_missing` — links in mails and CloudEvents name the wrong origin.
* `config.oidc_missing_in_multi_tenancy` — multi-tenant mode without SSO configured.
* `config.egress_allowlist_missing` / `config.egress_private_networks_allowed` — the outbound
  guard is wider than it should be (T-07).

Fix the variable in the deployment's environment and restart; the counter is per-process and
resets to the new startup's findings.

## Diagnostic queries

```promql
hubtask_config_invalid_total
```

## Escalation

None — but `egress_private_networks_allowed` in provider operation is a security posture
question, not a preference; treat that one as a finding for the security review.

## Follow-up

An installation that runs for weeks with a warning it means to keep (say, no SMTP on a dev
stack) should silence A-14 in its Alertmanager rather than here: the shipped rule states the
intended posture.
