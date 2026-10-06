---
id: UC-AI-07
title: Keep AI use within a budget
context: suggestion
actors: [PE-operator, PE-owner, PE-admin]
deployments: [D4, D5, D6]
serves: [P-03, P-11, P-15]
state: partial
tasks: [J-15]
checked_by: [core/application/service/integration/AiBudget_test.go]
---

# Keep AI use within a budget

## Goal

An operator — or a workspace for its own model — caps how much AI a workspace uses per day; when
the cap is reached, AI pauses with a sentence and everything else keeps working.

## Story

The installation's default is 200 000 tokens a day for the offered model. At the cap, *Suggest*
says "Today's AI budget is used up; it renews at midnight (workspace time)". The workspace sees its
use and the cap on its limits screen.

## How to check

1. The budget is a limit like any other: default at the installation, exception per workspace,
   later per plan (UC-INS-10).
2. Use is counted per workspace, per day in the workspace's time zone, **per source** (offered or
   own).
3. At the cap, AI requests are refused with a sentence naming the budget and when it renews;
   nothing else is affected.
4. The workspace sees use and cap side by side before reaching it.

## Where it ends

* No cost in money; tokens only ([NG-billing](../../vision/non-goals.md)).

## Today

* Check 2: not met in part — use is counted per workspace and day, without telling an offered model from an own one, tracked in #1068.
