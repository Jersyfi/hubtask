---
id: UC-FLW-02
title: Describe how our work moves, and switch it on
context: workflow
actors: [PE-admin, PE-member]
deployments: [D3, D4, D5, D6, D7]
serves: [P-03, P-04, P-05, P-06, P-11, P-13, P-17]
state: built
tasks: []
checked_by: [core/domain/model/work/Workflow_test.go, core/application/service/work/TakeStep_test.go, core/application/service/workflow/Workflows_test.go, core/application/service/sync/Patch_test.go, test/integration/workflow_test.go, apps/webapp/src/lib/data/workflows.test.ts]
---

# Describe how our work moves, and switch it on

## Goal

A team draws its process — the statuses its work passes through and the steps between them — tries
it, and puts it in force for chosen collections, starting from a default that already works.

## Story

The service team opens the designer and starts from the shipped "To do → In progress → Done". It
adds "Waiting for customer" in the *in progress* category and "Rejected" as a second *done*
status, draws a step from any status to "Rejected", and saves a draft. The preview says that the
board's columns will become the four statuses and that 37 open entries start in "To do". The team
publishes; the board now shows one column per status.

## How to check

1. A workflow has statuses, each in one category (backlog, to do, in progress, done), with a name
   and a colour from the token set, and steps (transitions) from one or more statuses — or from any
   — to one status.
2. The designer draws statuses and steps as a diagram and offers the same as a keyboard-operable
   list; every action in the diagram has a list equivalent.
3. A workflow is saved as a draft that changes nothing until it is published.
4. Publishing for a collection shows beforehand which status every existing entry will get and how
   the board's columns change; nothing moves without that confirmation.
5. A collection shows which workflow it uses and links to the one place it is defined.
6. Entering a done-category status completes the entry and leaving one reopens it, in the history
   with who did it.
7. A shipped default workflow exists and works without editing.
8. A person without the right to shape the workspace is not offered the designer.

## Where it ends

* One workflow per type per collection; no parallel statuses.
* Steps with conditions, required fields and actions: [UC-FLW-04](./UC-FLW-04-ask-for-what-a-step-needs.md).
* Changing a workflow in use: [UC-FLW-05](./UC-FLW-05-change-a-workflow-in-use.md).
