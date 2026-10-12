---
id: UC-AI-07
title: Keep AI use within a budget
context: suggestion
actors: [PE-operator, PE-owner, PE-admin]
deployments: [D4, D5, D6]
serves: [P-03, P-11, P-15]
state: partial
tasks: [J-15, SC-12]
checked_by: [core/application/service/integration/AiBudget_test.go, core/application/service/quota/Quota_test.go, test/integration/ai_budget_source_test.go, core/application/service/admin/Quotas_test.go, apps/webapp/e2e/aisettings.test.mjs]
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

* Check 3: not met — a spent budget answers like an absent provider, naming neither the budget nor when it renews, tracked in #1294.
