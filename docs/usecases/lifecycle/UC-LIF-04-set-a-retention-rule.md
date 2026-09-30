---
id: UC-LIF-04
title: Set a retention rule and see what it would do before it acts
context: lifecycle
actors: [PE-owner, PE-admin, PE-auditor, PE-scripter]
deployments: [D3, D4, D5, D6, D7]
serves: [P-04, P-06, P-07, P-08, P-11]
state: partial
tasks: [E-07, G-06, G-12, F4-18]
checked_by: [core/application/service/lifecycle/RetentionRules_test.go, core/application/service/lifecycle/RetentionRuleLifecycle_test.go, test/integration/retention_rule_test.go, test/retention/retention_test.go, cmd/hubctl/Retention_test.go]
---

# Set a retention rule and see what it would do before it acts

## Goal

A workspace that must not keep things forever — or must keep them for a set time — writes that down
once as a rule ("completed tasks: archive after a year, delete two years later"), sees exactly what
the rule would affect before it does anything, and can stop it at any time.

## Story

The office's owner opens *Retention* in the administration area and writes a rule for
completed tasks: after 365 days, archive; 730 days after that, delete. The rule is saved switched off. A
preview says how many entries it would affect today and shows some of them; it writes nothing. The
owner switches it on. Had the rule matched more than a twentieth of the workspace on its first run,
it would have been stored as *notify only* instead, with a notice saying so. For any hub or
collection the screen answers "which rules apply here, and where does each come from?". The owner
can switch a rule off, turn it into *notify only*, or withdraw it; withdrawing it takes back every
announcement it had made.

## How to check

1. Only the owner can write, change or withdraw a rule; an administrator or an auditor can read the
   rules and run a preview.
2. A rule names a scope (the workspace, a hub or a collection), a kind of data, a period, an action,
   and optionally a second stage, a grace period, whom to warn and a condition; the narrower scope
   wins over the wider one.
3. A period below the kind's lower bound is refused with `lifecycle.below_lower_bound`, naming the
   bound; one above the installation's upper bound needs a justification
   (`lifecycle.justification_required`), which goes into the trail.
4. A kind nothing removes yet is refused with `lifecycle.data_kind_not_swept` rather than stored.
5. A preview returns the count and examples of what the rule would affect and changes nothing.
6. A rule whose first run would affect more than 5 % of the holdings is stored as *notify only*,
   and the answer says so.
7. For a given hub or collection, the effective rules are listed with where each comes from and
   whether it is in force there.
8. Switching a rule off, making it *notify only* or withdrawing it stops it before its next action;
   withdrawing it clears the announcements it had made. Every change writes
   `lifecycle.rule_changed` or `lifecycle.rule_withdrawn`.
9. All of this — including scope, grace period, warning and second stage — can be set in the web
   app, through the API, with `hubctl` and through MCP.

## Where it ends

* No rule that anonymises work yet; anonymising belongs to erasure (privacy context).
* No hand-written scripts or regular expressions as conditions; conditions are the same expression
  language as automation ([ADR-0009](../../adr/ADR-0009-automation-rules-cel.md)).
* No installation-wide default rules or locks set by an operator; the operator's only say is the
  upper bound per kind.
* D1 and D2 need no rule: the trash period is enough (P-10).

See [data-retention.md](../../architecture/data-retention.md) and
[ADR-0020](../../adr/ADR-0020-retention-policies.md).

## Today

* **Check 9 fails in the web app and in part for `hubctl`.** The web form writes workspace-wide
  rules only, with kind, period, action and justification — no hub or collection scope, no grace
  period, no warning, no second stage, no condition (`apps/webapp/src/views/RetentionView.svelte`).
  `hubctl retention add` has no grace or warning flags; `set` has `--grace` only.
