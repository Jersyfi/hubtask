---
id: UC-AUT-08
title: Find rules that need attention before they fail
context: automation
actors: [PE-admin, PE-owner]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-11, P-12]
state: built
tasks: [F8-03, F8-07, F8-19, F8-25]
checked_by: [core/application/service/automation/Check_test.go, apps/webapp/src/lib/automation/findings.test.ts]
---

# Find rules that need attention before they fail

## Goal

When something a rule points at goes away — a label deleted, a collection removed, an account
switched off, an action no longer offered after an update — the rule's owner learns it when they
next look, not from five failures at three in the morning; and a rule that cannot run at all is
switched off before it tries.

## Story

The administrator opens *Rules*. The list checks every rule against the workspace as it is now. One
rule carries a note: *the label this step points at no longer exists*. Another is marked broken —
the account it runs as was removed — and has been switched off; its author got a mail. The
administrator opens the first rule; the note sits on the card it concerns. They pick another label
and save; the check runs again and the note is gone.

## How to check

1. Opening the rules list checks every rule of the workspace and shows, on each rule, what it
   found: *needs attention* (the rule runs, but a step would find nothing) or *broken* (the rule
   cannot run).
2. Deleting a label, a board column or a container checks the workspace's rules again without
   anybody opening the list.
3. A rule found broken is switched off, the trail holds `automation.rule_disabled` with the reason
   *check*, and its author is told by mail unless they switched integration messages off.
4. The check names each of these by its own sentence: an event this version does not publish, an
   action it does not offer, a parameter the action does not take, a required parameter nothing
   supplies, a condition that no longer compiles, an acting account that is gone or holds no role
   at the rule's scope, and a label, column, container, template, subscription, group or account
   that no longer exists.
5. Each finding points at the step or condition it is about, and the editor shows it on that card.
6. Saving an edit clears the rule's findings, and the editor asks for the check again at once.
7. The check repairs nothing and never runs across workspaces.

## Where it ends

* The check does not re-ask whether the writer may still delegate to the acting account; switching
  the rule on asks that, and every run asks it again per action ([ADR-0060](../../adr/ADR-0060-rule-check.md)).
* No check on a timer.
