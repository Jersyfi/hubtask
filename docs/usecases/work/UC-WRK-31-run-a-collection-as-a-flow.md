---
id: UC-WRK-31
title: Run a collection as a flow
context: work
actors: [PE-admin, PE-member]
deployments: [D3, D4, D5, D6, D7]
serves: [P-07, P-11]
state: specified
tasks: [UX-26]
checked_by: []
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

## Today

* Check 1: not met — not built yet, tracked in milestone UX.
* Check 2: not met — not built yet, tracked in milestone UX.
* Check 3: not met — not built yet, tracked in milestone UX.
* Check 4: not met — not built yet, tracked in milestone UX.
