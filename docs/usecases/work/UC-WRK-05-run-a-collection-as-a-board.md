---
id: UC-WRK-05
title: Run a collection as a board
context: work
actors: [PE-member, PE-person, PE-admin, PE-integrator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-06, P-08, P-11, P-13]
state: partial
tasks: [B-09, F2-11, F2-12, F9-07, F10-11]
checked_by: [test/integration/structure_test.go, core/domain/model/work/Bucket_test.go, apps/webapp/e2e/board.test.mjs, apps/webapp/e2e/container.test.mjs]
---

# Run a collection as a board

## Goal

A team sees its work as columns ("To do", "Doing", "Done"), moves a card from one column to the
next by hand or by keyboard, sees when a column holds more than agreed, and sets the columns up
itself.

## Story

The club's committee switches the "Summer fair" collection to the board. Whoever shapes the
collection adds the columns, gives "Doing" a limit of three and marks "Done" as the finishing
column. Members carry cards across; a fourth card in "Doing" turns the column's count into a
warning, and the card still lands. A card dropped in "Done" is completed. Somebody using the
keyboard moves a card with the arrow keys or the card's menu; on a phone the board shows one
column at a time.

## How to check

1. A person who may change the collection's shape creates, renames and deletes columns, sets a
   work-in-progress limit and marks one column as the done column; deleting a column asks first
   and says that its cards are not deleted but fall into the first column.
2. The same person changes the order of the columns, and the new order is what every member sees.
3. A card is carried with the pointer within a column and to another column; the move is kept
   after a reload and appears on other members' boards without a reload.
4. Every drag has a single-pointer and keyboard alternative: arrow keys rank a card, Shift with an
   arrow sends it to the top or the bottom, and the card's menu offers *Move to column …*.
5. A column holding more cards than its limit shows the count as a warning; the drop is **not**
   refused.
6. In the web app, a card dropped in the done column is completed, and that completion is in the
   card's history.
7. A member without the right to change the shape is not offered to create, edit or delete
   columns.
8. At phone width the board shows one column at a time, switched by a strip or a swipe, and the
   page never scrolls sideways.

## Where it ends

* The board shows tasks only; work packages and activities live under their task
  ([domain model §2](../../architecture/domain-model.md), capability `BUCKET`).
* A limit is advice, not a lock — by design (`core/domain/model/work/Bucket.go`).
* Taking a card out of the done column does not reopen it; that is a person's decision.
* The server completes nothing because of a column: a card moved to the done column through the
  API stays open unless an automation rule says otherwise (`core/domain/model/work/Bucket.go`).
* Swimlanes, and grouping a board by something other than its columns, are the saved view's
  grouping ([UC-WRK-17](./UC-WRK-17-filter-sort-and-save-a-view-of-my-work.md)).

## Today

* Check 2: not met in the web app — the board never reorders columns; they stay in creation order unless reordered through the API.
* Check 7: not met — *Add column*, *Edit column* and *Delete column* are drawn for everybody who sees the board; only the server's refusal stops a member.
