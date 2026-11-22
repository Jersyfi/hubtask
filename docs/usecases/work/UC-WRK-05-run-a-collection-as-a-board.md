---
id: UC-WRK-05
title: Run a collection as a board
context: work
actors: [PE-member, PE-person, PE-admin, PE-integrator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-06, P-08, P-11, P-13]
state: partial
tasks: [B-09, F2-11, F2-12, F9-07, F10-11]
checked_by: [test/integration/structure_test.go, core/domain/model/work/Bucket_test.go, apps/webapp/e2e/board.test.mjs, apps/webapp/e2e/container.test.mjs, apps/webapp/e2e/entrybyrole.test.mjs, apps/webapp/e2e/boardlevels.test.mjs, apps/webapp/src/lib/entries/boardlevel.test.ts, apps/webapp/e2e/entryhead.test.mjs, apps/webapp/e2e/viewpanel.test.mjs, apps/webapp/src/lib/workspace/unusedColumns.test.ts, core/application/service/work/SwitchBoard_test.go]
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
6. A card moved into the done column - by a drop, the API, a sync push or a rule - is completed,
   and moved out of it is reopened; completing a card moves it into the done column and reopening
   moves it out into the first column. The server keeps column and completion together through
   every door, and each change is in the card's history. A board without a workflow has exactly
   one done column (owner's decision of 2026-11-12).
7. A member without the right to change the shape is not offered to create, edit or delete
   columns.
8. At phone width the board shows one column at a time, switched by a strip or a swipe, and the
   page never scrolls sideways.
9. On a board under a workflow each column shows one or more statuses; a person who may change
   the shape maps them, and the statuses no column shows are in a folded column *Ohne Spalte*
   that accepts drops (owner's decision of 2026-11-12).
10. Under a workflow a card dropped on a column takes the one step that reaches it; where several
    do, the person picks one in a picker and nothing changes until they do; where none does, the
    drop is refused and the card shows why (owner's decision of 2026-11-12).
11. An entry's state is changed by one control, the same on the list row, the card, the entry head
    and the phone sheet: the checkbox without columns, the column chip on a board without a
    workflow, the status chip where a workflow runs. A person who may change the collection's shape
    shows a list as a board and a board without a workflow as a list again from the collection's
    ⋯ menu, after a preview: a board starts with an open and a done column, completed cards in the
    done one; back to a list the columns go and every entry keeps its completion. Neither is
    offered while a workflow runs for the tasks (owner's decision of 2026-11-12).
12. A board may be drawn in lanes by a second field - a person, a label's or a custom field's
    value -, each lane a row of the same columns with its count, the cards without a value in the
    last lane; the lanes are saved with the view as `grouping.lanes_by`, held to the grouping's
    rule (E-21).
13. A collection that has columns but offers no board says so in its settings with the number of
    columns and offers *Remove columns*; the confirmation says how many columns go, that every
    entry keeps whether it is done, and that it cannot be undone; after confirming the collection
    has no columns and no entry changed its completion (owner's decision of 2026-11-22).

## Where it ends

* The board shows tasks only; work packages and activities live under their task
  ([domain model §2](../../architecture/domain-model.md), capability `BUCKET`).
* A limit is advice, not a lock — by design (`core/domain/model/work/Bucket.go`).
* Column and completion are one fact on a board without a workflow: taking a card out of the done
  column reopens it, and the server couples them through every door
  ([domain model §3.5a](../../architecture/domain-model.md), owner's decision of 2026-11-12).
* Swimlanes, and grouping a board by something other than its columns, are the saved view's
  grouping ([UC-WRK-17](./UC-WRK-17-filter-sort-and-save-a-view-of-my-work.md)).

## Today

* Check 2: not met in the web app — the board never reorders columns; they stay in creation order unless reordered through the API.
* Check 9: partly met — each column names its statuses and *Ohne Spalte* stands folded and takes drops; the mapping is changed per status in the scheme change's preview, not yet walked on screen, and not on the board itself.
