---
id: UC-LIF-03
title: Archive finished work and bring it back when needed
context: lifecycle
actors: [PE-person, PE-member, PE-admin, PE-scripter, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-05, P-08, P-11]
state: partial
tasks: [B-06, B-10, F2-14, F10-03]
checked_by: [core/application/service/work/ArchiveWorkItem_test.go, core/application/service/work/ArchiveContainer_test.go, test/integration/container_lifecycle_test.go, apps/webapp/e2e/archive.test.mjs]
---

# Archive finished work and bring it back when needed

## Goal

A person puts finished work out of the way without deleting it: an archived task, collection or hub
leaves the everyday lists, stays findable and readable for as long as the workspace keeps it, and
comes back with one action.

## Story

At the end of the school year a parent archives the *Homework 2025/26* collection. It disappears
from the sidebar and from the lists, and everything in it is archived with it. In the *Archive*
place they see what is archived and can bring any of it back. A single finished task can be archived
from its menu in the same way. Nothing archived is ever removed unless a retention rule the
workspace wrote says so.

## How to check

1. A person who may edit a task can archive and unarchive it; an administrator can archive and
   unarchive a collection or a hub.
2. Archived things leave the everyday lists and remain readable; a list can be asked to include
   them.
3. Everything under an archived collection or hub counts as archived until the container is
   unarchived.
4. An archived task cannot be changed; a change is refused with `items.archived`, which says to
   unarchive it first.
5. The *Archive* place lists archived hubs, collections **and** tasks, and brings each back.
6. Archiving and unarchiving are possible in the web app, through the API, with `hubctl` and
   through MCP.
7. An archived thing is never removed except by a retention rule for archived work; without one it
   stays.
8. Each archive and unarchive writes an entry in the trail.

## Where it ends

* An archive is not a backup and not an export; it is a state inside the workspace.
* No automatic archiving without a rule the workspace wrote (see *Set a retention rule*).
* No read-only sharing of archived work with outsiders.

## Today

* Check 5: not met — the *Archive* place lists hubs and collections only; an archived task is not listed, because there is no workspace-wide query for archived entries.
* Check 6: not met for `hubctl` — it has no archive or unarchive command, only `--include-archived` on listings.
