---
id: UC-AUT-02
title: Try a rule before it acts
context: automation
actors: [PE-admin, PE-owner, PE-member, PE-scripter, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-04, P-08, P-11]
state: built
tasks: [G-09, F4-14, F8-06, F8-26]
checked_by: [core/application/service/automation/TestRule_test.go, apps/webapp/src/lib/automation/probe.test.ts]
---

# Try a rule before it acts

## Goal

Before a rule is let loose, its writer sees what it would do to a real example — which conditions
hold, which actions would run — and nothing in the workspace changes while they look.

## Story

In the editor the administrator opens the probe, picks a sample event (*an entry is completed*) and
an entry to play it against, and presses *Try it*. The probe shows each condition with its answer
and each action marked *would run* or *would not run*, both arms of every branch included. They
adjust the condition and try again — the probe tests the rule as it stands on the canvas, not as it
was last saved. Before the probe even runs, the editor names what is missing: no event chosen, a
required parameter nothing fills, a branch with two empty arms.

## How to check

1. A dry run against a sample event answers, per condition, whether it held, and per action whether
   it would run — for both arms of every branch.
2. A dry run writes nothing: no entry changes, no run is recorded, no delivery or call is made.
3. A dry run can test an unsaved definition as well as a stored rule; asking for neither or both is
   refused with `automation.test_source_required`.
4. A sample without an event type is refused with `automation.test_event_type_required`.
5. The editor names a missing event, a required parameter nothing fills and a branch with two empty
   arms at the card they concern, before the probe is pressed; these notes block nothing.
6. The dry run needs the automation permission at the rule's scope; `hubctl rule test` and the API
   give the same answer as the editor.

## Where it ends

* A dry run evaluates the rule; it does not predict whether the acting account's rights will hold
  on the day it runs — the run decides that, per action.
* No dry run of an outside call's answer: a rule never reads one.
* No replaying a week of real events through a draft.
