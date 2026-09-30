---
id: UC-WRK-06
title: Put work in order and move it where it belongs
context: work
actors: [PE-person, PE-member, PE-agent, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-08, P-11, P-12, P-13]
state: built
tasks: [B-08, F2-12, F10-11]
checked_by: [test/integration/move_test.go, core/domain/model/work/Path_test.go, apps/webapp/e2e/container.test.mjs]
---

# Put work in order and move it where it belongs

## Goal

A person ranks their list the way they mean to work through it, lifts a step out from under its
task or tucks one under another, and moves an entry to the collection where it really belongs —
and is told plainly what could not come along.

## Story

The person drags "Call the plumber" to the top of the list, or uses the row's menu: *Move up*,
*Move to top*, *Move inside the row above*, *Move out*. An entry filed in the wrong place goes
*Elsewhere…* to another collection, in the same hub or another one. Before the move the dialog
warns that labels and the column may not exist there; afterwards it lists what was left behind,
by kind.

## How to check

1. A row is dragged within its own level to a new rank; the same is possible from the row's menu
   (*up*, *down*, *top*, *bottom*) and by keyboard, with no pointer.
2. The new order survives a reload and is the same on another device and for other members.
3. *Move inside the row above* makes the entry a child of that row, and *Move out* lifts it one
   level — each only where the capability profile permits that type at that level; otherwise the
   move is refused with `items.parent_type_invalid` or `items.depth_exceeded`.
4. Moving an entry into itself or into anything under it is refused with `items.parent_is_self` or
   `items.parent_in_own_subtree`.
5. *Elsewhere…* offers every collection the person can see that is not archived and not the
   current one; the entry and everything under it arrive at the top level of the destination.
6. After a move to another collection, every label and column that does not exist there is
   removed from the entry and **listed to the person by kind** — nothing is dropped silently.
7. The history of the entry records `item.moved` for a move and `item.reordered` for a change of
   rank, and a rule can react to either.

## Where it ends

* Moving a whole collection to another hub is [UC-WRK-01](./UC-WRK-01-organise-my-work-into-hubs-and-collections.md).
* Moving to another workspace is not possible and not planned ([P-01](../../vision/principles.md)).
* No manual sort that is private to one person: the rank is the collection's. A personal order is
  a saved view's sort ([UC-WRK-17](./UC-WRK-17-filter-sort-and-save-a-view-of-my-work.md)).
* Moving many entries at once is [UC-WRK-08](./UC-WRK-08-change-many-entries-at-once.md).
