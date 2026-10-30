---
id: UC-SVC-09
title: Approve or decline, with a reason
context: servicedesk
actors: [PE-handler, PE-requester]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-11, P-18]
state: built
tasks: []
checked_by: [core/domain/model/servicedesk/Approval_test.go, core/application/service/servicedesk/Approvals_test.go, test/integration/service_desk_approval_test.go, core/application/service/notification/RecordApproval_test.go, apps/webapp/src/lib/servicedesk/approvals.test.ts]
---

# Approve or decline, with a reason

## Goal

Somebody asked for their yes finds what waits for them, approves or declines with a reason, and everybody who reads the entry sees who decided what, when and on which version of it.

## Story

Mehmet is the manager named on Jonas's laptop request. His list shows it waiting. He approves; the next level, finance, is asked. Anna of finance declines: "Over this quarter's budget." The request moves to *Declined*, and its trail names Mehmet's yes and Anna's no with her reason and the version of the request they saw.

## How to check

1. A person lists the approvals waiting for them: those whose current level counts them or names a role they hold there, oldest first, each with the entry's title and number.
2. They approve or decline at the current level; a decline without a reason is `approvals.reason_required`, a second answer at the level is `approvals.already_decided`, and somebody who is no approver is told the approval is not found.
3. An entry's readers read its approvals, each with its levels, the people counted, where it stands and its trail: who decided, for whom, when, why and on which version of the entry.
4. The people a level begins to count are told by mail that it waits for their yes, with a link to the app's *Waiting for my yes* that opens the app and signs nobody in (P-02); there they approve, or decline with a reason, and an entry's readers see its approvals and trail beside it.

## Where it ends

* Deciding from the requester portal's own screens is the next part; an approver decides in the app.
