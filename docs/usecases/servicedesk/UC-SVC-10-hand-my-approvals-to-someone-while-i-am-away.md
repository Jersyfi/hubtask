---
id: UC-SVC-10
title: Hand my approvals to someone while I am away
context: servicedesk
actors: [PE-handler]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-11, P-18]
state: built
tasks: []
checked_by: [core/domain/model/servicedesk/Approval_test.go, core/application/service/servicedesk/Approvals_test.go, test/integration/service_desk_approval_test.go]
---

# Hand my approvals to someone while I am away

## Goal

A person who will be away hands their approvals to somebody for that time, so nothing waits for them, and the trail still says who decided for whom.

## Story

Mehmet is on leave for two weeks and hands his approvals to Lena from Monday to the Friday after next. A request waiting for Mehmet appears on Lena's list; she approves it, and the trail says Lena decided for Mehmet. Back early, Mehmet ends the delegation and decides himself again.

## How to check

1. A person sets a delegation to somebody from one moment to another of at most a year; to themselves is `approvals.delegate_self`, an end before the start `approvals.delegation_empty`; a new one replaces the earlier one, and they read and end it.
2. While it is in force the delegate lists and decides the approvals counting the person away; the decision names both, and the person away cannot answer the same level again.

## Where it ends

* The web app's screen for a delegation is the next part.
