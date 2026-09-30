---
id: UC-AUT-03
title: Run a rule when I press a button
context: automation
actors: [PE-admin, PE-member, PE-scripter, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-08, P-11]
state: built
tasks: [G-08, F4-14]
checked_by: [core/application/service/automation/TriggerRule_test.go, test/integration/automation_trigger_test.go]
---

# Run a rule when I press a button

## Goal

A procedure the workspace runs now and then — close the month, prepare the weekly review — is one
press away, for whoever manages the rule, and every press is its own run with its own record.

## Story

The rule's trigger is *on demand*. On the runs page the administrator picks the rule and presses
*Run*; the page answers at once that the run was started, and a moment later the run is listed with
its result.
A script does the same with `hubctl rule trigger`, an agent through MCP.

## How to check

1. Pressing *Run* for an enabled on-demand rule starts a run and answers with that run's identifier
   before the run has finished.
2. Two presses are two runs; neither is skipped as a repeat of the other.
3. The run records who pressed.
4. A rule whose trigger is not *on demand* is refused with `automation.trigger_not_manual`; a rule
   that is switched off with `automation.rule_not_enabled`.
5. Pressing needs the automation permission at the rule's scope and nothing more; what each action
   may do is decided per action, as the rule's acting account, when it runs.
6. Everything the run then does is listed and replayable like any other run.

## Where it ends

* No parameters typed at the press: an on-demand rule acts on what its actions name.
* No waiting for the result in the request; the run is followed on the runs page.
