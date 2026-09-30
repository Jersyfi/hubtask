---
id: UC-WRK-04
title: Tick work off, and take it back
context: work
actors: [PE-person, PE-member, PE-child, PE-agent, PE-integrator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-08, P-11, P-12, P-13]
state: built
tasks: [B-07, F2-09, F6-13]
checked_by: [test/integration/completion_test.go, test/integration/rollup_test.go, core/domain/model/work/Completion_test.go, core/domain/model/work/CompletionPolicy_test.go]
---

# Tick work off, and take it back

## Goal

A person marks something done with one action, sees that it is done, and can undo it just as
easily. Where the collection says so, a parent finishes by itself when all its steps are done.

## Story

A child ticks "Empty the dishwasher" in the chores list; the title is struck through and a small
celebration plays — unless they switched celebrations off in their profile. Ticked by mistake,
the checkbox brings it back. In the renovation collection the family chose *roll-up*: when the
last activity under "Tiles" is done, "Tiles" completes itself, and reopening an activity reopens
it.

## How to check

1. Every entry type carries completion: a checkbox before the title completes it, and the same
   checkbox reopens it.
2. A completed entry shows its title struck through and records who completed it and when;
   reopening clears both.
3. The history shows `item.completed` and `item.reopened` as two different sentences.
4. With the collection's completion policy set to roll-up, completing the last open child
   completes the parent, and reopening any child reopens the parent; with the manual policy
   nothing is completed on anybody's behalf.
5. Completing twice, or reopening an open entry, changes nothing and writes no second history
   entry.
6. The celebration after a completion is not shown to a person who switched celebrations off.
7. Changing the completion policy is offered only to somebody who may change the collection's
   shape; others see why they cannot.
8. Completing an entry is possible through the API, MCP and an automation action, and each writes
   the same event a rule can react to.

## Where it ends

* Completing a parent does **not** complete its open children.
* No "percent done" on an entry beyond the count of completed first-level children.
* No required fields that block completion.
* A done column on the board completing a card is [UC-WRK-05](./UC-WRK-05-run-a-collection-as-a-board.md).
