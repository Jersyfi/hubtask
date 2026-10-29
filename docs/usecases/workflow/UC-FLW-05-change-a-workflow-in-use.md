---
id: UC-FLW-05
title: Change a workflow that is already in use
context: workflow
actors: [PE-admin]
deployments: [D3, D4, D5, D6, D7]
serves: [P-03, P-04, P-06, P-11]
state: built
tasks: []
checked_by: [core/domain/model/work/Workflow_test.go, core/application/service/work/TakeStep_test.go, core/application/service/workflow/Workflows_test.go, core/application/service/sync/Patch_test.go, test/integration/workflow_test.go, apps/webapp/src/lib/data/workflows.test.ts, core/application/service/work/SetItemEntryType_test.go]
---

# Change a workflow that is already in use

## Goal

A team improves its process while work is running, and no entry is stranded in a status that no
longer exists or moved without anybody knowing.

## Story

The service team removes "Waiting for customer". The draft shows that 12 entries are in it and asks
where they should go; the team picks "In progress". Publishing moves them in one change, writes it
into each entry's history as a workflow change, and the trail names who published.

## How to check

1. Editing a published workflow creates a draft; the published one stays in force until the draft
   is published.
2. Publishing a draft that removes or renames a status in use asks, per removed status, for the
   status its entries move to, with their count.
3. All moves of one publish happen together or not at all.
4. Each moved entry's history says the workflow changed and from which status to which.
5. The trail records the publish, who did it, and the counts — no entry title (rule 10).
6. A publish never runs a step's conditions or actions on the moved entries.

## Where it ends

* No version history of workflows beyond the trail; no rollback other than publishing the old shape.
