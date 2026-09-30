---
id: UC-AUT-07
title: Keep rules from running away
context: automation
actors: [PE-admin, PE-owner, PE-operator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-04, P-11]
state: built
tasks: [G-07, H-08, F8-22]
checked_by: [core/application/service/automation/RunRule_test.go, test/integration/automation_run_test.go, test/integration/notification_rule_test.go]
---

# Keep rules from running away

## Goal

A mistake in a rule — two rules that trigger each other, a rule that fires on every one of a
thousand bulk changes, a rule that fails every time — stops itself, tells its author, and never
drags down the rest of the workspace or another workspace.

## Story

Two rules each add a label that the other reacts to. After a few hops the chain stops, and the
runs page shows the last run as *Aborted (loop)*. A rule that creates a task per completed entry
meets a bulk completion of 500 entries; after its hourly allowance the remaining runs are
*Throttled*. A rule whose destination was deleted fails five times in a row; it switches itself off
and its author receives a mail saying so, with a link to the rule.

## How to check

1. A chain of rules triggering rules stops at depth 5: the run at the limit acts on nothing and is
   recorded as *Aborted (loop)*.
2. No rule reacts to the events of a rule run itself, and none reacts to events a restore
   replayed.
3. A rule with an hourly limit records every run past it as *Throttled* without evaluating its
   conditions; a workspace's own hourly budget for all its runs is enforced the same way.
4. A rule with a de-duplication key runs once for a burst of events that share the key's value.
5. After five failed runs in a row the rule switches itself off, the trail holds
   `automation.rule_disabled` with the reason, and the rule's author — not the account it runs as —
   is told by mail unless they switched integration messages off.
6. A skipped or throttled run ends a failure streak rather than adding to it.
7. A failing rule in one workspace causes no run, delay or refusal in another.
8. An action's failure under *continue on error* leaves the run *Succeeded*, with that action's
   failure listed; under *stop* the run ends there as *Failed*.

## Where it ends

* No automatic repair of a rule; switching it off is the whole reaction.
* The depth limit and the failure count are fixed; the throttle is set per rule and the hourly
  budget per workspace by its plan or operator.
