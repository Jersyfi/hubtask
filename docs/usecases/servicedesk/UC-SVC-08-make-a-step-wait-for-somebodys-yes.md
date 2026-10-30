---
id: UC-SVC-08
title: Make a step wait for somebody's yes
context: servicedesk
actors: [PE-handler, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-11, P-18]
state: built
tasks: []
checked_by: [core/domain/model/work/Workflow_test.go, core/domain/model/servicedesk/Approval_test.go, core/application/service/workflow/Approval_test.go, core/application/service/servicedesk/Approvals_test.go, infrastructure/postgres/WorkflowApproval_test.go, test/integration/service_desk_approval_test.go, core/application/service/servicedesk/ApprovalReminders_test.go, apps/webapp/src/lib/data/workflows.test.ts]
---

# Make a step wait for somebody's yes

## Goal

A team makes a step of its workflow wait until the right people said yes - one of them, all, a number or a majority, level after level - so that a purchase, an access or a change moves on only once it is approved.

## Story

Lena's workflow for purchases has a step *Approve* from *Asked* to *Ordered*. She makes it wait for the requester's manager first, then for two of the finance group. Jonas takes the step on a request for a laptop; it stays in *Asked* until the manager and two of finance said yes, and then moves to *Ordered* by itself.

## How to check

1. A workflow's step names an approval: one to five levels, each with people, groups, roles or person fields and a rule - any, all, N of them or a majority -, a status a decline moves the entry to, and the hours each level has. A level naming nobody, an unknown rule, a role under all or a majority, or a decline status the workflow lacks is refused, each with its code.
2. A level passes by its rule over the people it counted when the approval opened - named, in its groups, named by its person fields -; a role is held along the entry's path when somebody decides.
3. Taking such a step opens an approval and leaves the entry where it stands; when the last level passes the step is taken, and one decline moves the entry to the decline status. Another step taken meanwhile withdraws the approval. One approval waits on an entry at a time.
4. Approvals, their decisions and delegations are in the workspace archive and the access export, and stay in their workspace.
5. A level with hours reminds the people it counts who have not answered halfway through them, and at their end reminds them again and tells the person who asked that it is overdue; nothing is decided for anybody, and a level passed, an approval decided or withdrawn stops its clocks.
6. The workflow designer makes a step wait for an approval and writes its levels - people, groups, roles, person fields, the rule and N -, the decline status and the hours each level has.

## Where it ends

* A level's hours are wall hours; counting them on a business calendar is the next release's.
* A group named by a template cannot be carried to another workspace: groups have no stable key, so a template's approval names a role.
