---
id: UC-INS-10
title: Set limits for workspaces, and see them before they hurt
context: admin
actors: [PE-operator, PE-owner, PE-admin]
deployments: [D5, D6, D7]
serves: [P-03, P-07, P-11]
state: built
tasks: [H-08, SI-17]
checked_by: [core/application/service/admin/Quotas_test.go]
---

# Set limits for workspaces, and see them before they hurt

## Goal

An operator sets ceilings — entries, storage, requests, automation runs, AI budget — for every
workspace and exceptions for single ones; a workspace sees its limits and its use before it hits
one, and hitting one never deletes anything.

## Story

The installation's defaults carry the ceilings; *Installation → Workspaces → Limits* sets an
exception for one workspace, showing the default as the placeholder. In the workspace,
*Administration → Limits* shows each limit, the use so far and where the limit comes from. At the
limit, the next new entry is refused with a sentence naming the limit; everything that exists stays.

## How to check

1. The effective limit resolves product → installation → (plan) → workspace exception, and the
   workspace screen says which level set it.
2. The exception dialog shows the installation's value as the placeholder; empty means "use the
   default".
3. Reaching a limit refuses the next creation with the limit named; nothing existing is removed or
   hidden.
4. A workspace's administrators see use and limit side by side; they cannot change the limit.

## Where it ends

* No warnings by mail at 80 %; telling a platform is UC-INS-14.
* No prices ([NG-billing](../../vision/non-goals.md)).
