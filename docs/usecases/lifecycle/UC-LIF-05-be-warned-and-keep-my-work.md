---
id: UC-LIF-05
title: Be warned before a rule removes my work, and keep it
context: lifecycle
actors: [PE-member, PE-admin, PE-scripter]
deployments: [D3, D4, D5, D6, D7]
serves: [P-03, P-04, P-08, P-11]
state: partial
tasks: [E-07, G-12, F4-18]
checked_by: [core/application/service/lifecycle/RetainItem_test.go, core/application/service/lifecycle/RetentionSweep_test.go, test/retention/retention_test.go]
---

# Be warned before a rule removes my work, and keep it

## Goal

Nobody's work is removed by a rule without warning: the people it concerns are told when it is
announced, can see on the task itself what will happen and when, and can take it out of the rule
during the grace period.

## Story

A rule will delete completed tasks two years old. When the rule's pass reaches one of them, it marks
the task and its members receive a *retention* message saying what will happen and on which day —
a message their notification preferences cannot silence. On the task itself they see *will be
deleted on 14 October* and a *Keep it* action. Choosing it takes the task out of this announcement;
editing or moving the task does the same. If nobody acts, the action happens when the grace period
ends. A task that a legal hold protects is not announced at all and shows what is holding it.

## How to check

1. When a rule marks a task, its members (and, where the rule says so, the collection's or
   workspace's administrators) receive one retention message each, naming the action and the day;
   somebody who qualifies twice is told once.
2. The retention message cannot be switched off by a notification preference.
3. The task shows, in every client, the announced action, the day it takes effect, and whether the
   reader may keep it.
4. A person who may edit the task can keep it during the grace period, in the web app, through the
   API, with `hubctl retention retain` and through MCP; keeping a task that is not announced is
   refused with `lifecycle.not_marked`.
5. Editing or moving an announced task also takes it out of the announcement.
6. A task kept this way is not acted on at the end of the grace period; a later pass judges it
   afresh.
7. A task that a legal hold protects is not announced and shows what is holding it.
8. Keeping a task writes `lifecycle.retained`.

## Where it ends

* Keeping a task is not a permanent exemption; a permanent exemption is a condition in the rule or
  a legal hold.
* No warning for kinds that have no marking phase (notifications, sessions, the trash itself);
  those go at the end of their period.

See [data-retention.md](../../architecture/data-retention.md) §5–6.

## Today

* **Checks 3 and 7 fail in the web app.** The server answers every entry's `retention` state, with
  what blocks it, but the web app never shows it.
* **Check 4 fails in the web app.** The data layer has the call (`apps/webapp/src/lib/data/policies.svelte.ts`,
  `retain`), but no screen offers it.
