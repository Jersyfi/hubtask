---
id: UC-WRK-31
title: Run a collection as a flow
context: work
actors: [PE-admin, PE-member]
deployments: [D3, D4, D5, D6, D7]
serves: [P-07, P-11]
state: verified
tasks: [UX-26]
checked_by: [core/domain/model/work/Flow_test.go, core/domain/model/work/StartingColumns_test.go, test/integration/starting_columns_test.go, core/domain/model/work/Figure_test.go, core/application/service/work/FlowGate_test.go, core/application/service/work/FigureReader_test.go, test/integration/flow_gate_test.go, test/integration/figure_repository_test.go, apps/webapp/src/lib/data/figure.test.ts, apps/webapp/e2e/flow.test.mjs, apps/webapp/e2e/figure.test.mjs]
---

# Run a collection as a flow

## Goal

Requests, orders or onboardings move through named states with limits, and the collection says where work piles up.

## Story

The support team's "Requests" collection uses *Flow*: columns *New*, *In progress*, *Waiting*, *Done*, a limit of three in progress, the field "Customer" required. New mail arrives through the jumble and is handed out in turn.

## How to check

1. The preset *Flow* makes the board the default view with columns as states, a limit per column, and a column that completes entries.
2. A required field must be set before an entry may enter a column after the first; the move is refused with the field named.
3. A column over its limit is marked on the board, in the navigation and in the collection's figure; moving into it is still possible and announced.
4. The collection's figure shows entries completed per week and the average time from first to last column.

## Where it ends

* The server enforces nothing about limits beyond reporting them ([UC-WRK-05](./UC-WRK-05-run-a-collection-as-a-board.md)).
* No approvals or sign-offs.
