<!-- SPDX-License-Identifier: BUSL-1.1 -->
# RB-A16 — An automation rule switched itself off

**Alert:** `HubtaskRuleSelfDisabled` · **Severity:** ticket · **Catalogue:** A-16

## The symptom

The engine's self-protection fired: a rule failed enough runs in a row
(`consecutive_failures`) to be disabled. The rule's owner has already been notified through the
normal notification path — this ticket is not for telling them, it is for finding out what kept
failing, because a pattern of self-disables is a product defect wearing a user's clothes.

## Immediate action

```bash
# Which rules, and on what error? (as the owner role)
psql -c "SELECT r.tenant_id, rr.rule_id, rr.error_code, count(*)
         FROM rule_run rr JOIN automation_rule r ON r.id = rr.rule_id
         WHERE rr.status = 'FAILED' AND rr.started_at > now() - interval '2 hours'
         GROUP BY 1, 2, 3 ORDER BY count(*) DESC LIMIT 10;"
```

* **`access.not_permitted`** → the rule's `run_as` account lost a permission it needs; the rule
  was authored against a capability somebody since revoked. The owner fixes this; nothing to do.
* **`automation.action_failed` against one action kind across tenants** → that action's handler
  has a defect; this is ours.
* **An outbound action against a dead target** → the breaker and the disable did their jobs.

## Diagnostic queries

```promql
increase(hubtask_rule_disabled_total[24h])
sum by (result) (rate(hubtask_rule_runs_total[1h]))
```

## Escalation

Multiple tenants' rules disabling on the same error code in one day is a regression in the
engine or an action handler — treat as a defect with SLO-7 exposure, not as user error.

## Follow-up

SLO-7 counts the share of runs without an internal error; check the dashboard's automation row
after any engine fix to confirm the failure share fell back.
