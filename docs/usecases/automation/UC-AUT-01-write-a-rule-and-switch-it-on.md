---
id: UC-AUT-01
title: Write a rule and switch it on
context: automation
actors: [PE-admin, PE-owner, PE-member, PE-scripter, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-07, P-08, P-11, P-12]
state: built
tasks: [G-05, G-06, G-07, G-09, F4-13, F8-04, F8-16, F8-17, F8-18, F8-22]
checked_by: [core/application/service/automation/Rules_test.go, test/integration/automation_rule_test.go, apps/webapp/e2e/rules.test.mjs]
---

# Write a rule and switch it on

## Goal

Somebody who repeats the same steps by hand writes them down once — *when this happens, if that
holds, do these things* — and the workspace does them from then on, never with more rights than
the writer has.

## Story

In *Administration → Rules* the administrator starts a new rule. The editor draws it as a path: a
trigger, a gate holding the conditions, the chain of actions. They pick *an entry is completed* as
the trigger, compose a condition as a sentence (*the entry is in collection Invoices*), add *add
label Paid* and *add a comment*, choose the service account it runs as, and save. The rule is
saved switched off; the list shows it with the sentence the rule reads as. They read it back and
press *Switch on*. From then on every matching completion is handled.

A scripter does the same with `hubctl rule add` or the API; an agent through MCP.

## How to check

1. A newly saved rule is switched off, whatever its trigger; switching it on is a separate act with
   its own trail entry (`automation.rule_enabled`).
2. Writing a rule needs the automation permission at the rule's scope (the workspace, a hub or a
   collection); anybody else is refused as not permitted.
3. A rule may run as a service account or as the writer; naming a colleague is refused with
   `automation.run_as_not_delegable`, an account that can do more at that scope than the writer
   with `automation.run_as_exceeds_writer`, a switched-off account with
   `automation.run_as_inactive`.
4. A rule with an action the writer could not perform themselves is refused with
   `automation.writer_lacks_action_right`, naming the scope that is missing.
5. A condition that does not compile is refused at the save with `automation.condition_invalid`,
   pointing at the condition; an action kind that does not exist with `automation.action_unknown`;
   a parameter the action does not take is refused by name.
6. Switching a rule on asks checks 2–4 again, against the writer's rights on that day.
7. Two people editing one rule: the second save is refused with `automation.version_conflict` and
   nothing is overwritten.
8. Switching a rule off or deleting it never needs more than the automation permission — anybody
   who may manage rules there can always stop one.
9. Every action a use case offers is available as a rule action, and the editor builds its forms
   from what the installation declares, so an action added in a later version appears without a
   client release.

## Where it ends

* Conditions are CEL expressions or the sentences that produce them; no scripting language, no
  loops, no reading external data in a condition ([ADR-0009](../../adr/ADR-0009-automation-rules-cel.md)).
* Rules are not written or switched offline; a device holds nothing of a rule.
* No rule applies to what happened before it was switched on.
* Recurring tasks are a property of the entry, not a rule; a rule may still set one.
