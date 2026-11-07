---
id: UC-AST-03
title: Merge duplicate configuration items without losing one
context: assets
actors: [PE-admin, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-18]
state: built
tasks: []
checked_by: [core/application/service/cmdb/Cmdb_test.go, apps/webapp/src/lib/data/cmdb.test.ts]
---

# Merge duplicate configuration items without losing one

## Goal

When one reported item matches several configuration items, nothing is written and a duplicate waits for a person, who merges them into one that keeps everything, or says they are distinct.

## Story

Two people had recorded the same laptop by hand under different names. The next night the endpoint manager reports it by its MAC address and finds both. Dana keeps the older record; it takes the other's serial number and relations, and the other stays in the history, retired as merged.

## How to check

1. An item that matches more than one CI by the same rule writes nothing, is reported as `DUPLICATE` with its candidates, and opens one duplicate for those CIs - not a second one while it is open.
2. Merging into a survivor keeps the survivor's identity and values, gives it the attributes it lacks, the others' source keys and their relations; each other CI is retired as merged into it and is never deleted.
3. Resolving without a survivor records the CIs as distinct and closes the duplicate.
4. A survivor that is not one of the duplicate's CIs is refused with `cmdb.survivor_not_in_duplicate`.
5. The *Assets* screen lists the open duplicates side by side and merges or separates them.

## Where it ends

* No automatic merge and no deletion: a person decides every merge.
