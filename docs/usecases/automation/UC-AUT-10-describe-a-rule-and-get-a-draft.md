---
id: UC-AUT-10
title: Describe a rule and get a draft
context: automation
actors: [PE-admin, PE-owner]
deployments: [D1, D3, D4, D5, D6]
serves: [P-11, P-14]
state: specified
tasks: []
checked_by: []
---

# Describe a rule and get a draft

## Goal

An administrator writes in a sentence what should happen and gets a rule drawn on the canvas, switched off, ready to probe.

## Story

"When a request arrives by mail, give it to Lisa and set it due in two days." The editor draws trigger, two actions and the explanation sentence; the probe runs with the last arrival.

## How to check

1. With AI consented to, the rule editor offers *Describe it*; without it nothing is shown.
2. The draft uses only triggers, conditions and actions the workspace offers and only people, collections and labels it was shown; it is created switched off.
3. The draft is shown on the canvas with its sentence and the probe offered at once; nothing runs until a person switches the rule on.
4. A description the engine cannot express is answered with what part could not be drafted.

## Where it ends

* The rule engine's limits stay [UC-AUT-07](./UC-AUT-07-keep-rules-from-running-away.md).

## Today

* Check 1: not met — not built yet, tracked in milestone UX.
* Check 2: not met — not built yet, tracked in milestone UX.
* Check 3: not met — not built yet, tracked in milestone UX.
* Check 4: not met — not built yet, tracked in milestone UX.
