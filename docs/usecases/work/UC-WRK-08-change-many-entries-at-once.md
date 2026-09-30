---
id: UC-WRK-08
title: Change many entries at once
context: work
actors: [PE-member, PE-person, PE-admin, PE-scripter, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-08, P-11, P-12, P-13]
state: built
tasks: [C-11, F3-11, F10-10]
checked_by: [test/integration/bulk_and_duplicate_test.go, core/application/service/work/BulkUpdateWorkItems_test.go, apps/webapp/e2e/container.test.mjs]
---

# Change many entries at once

## Goal

A person who has to do the same thing to twenty entries — tick them off, label them, hand them to
somebody, clear them into the trash — does it once, and learns exactly which ones worked and which
did not.

## Story

After a club event the organiser chooses *Select* from the page menu, ticks the fourteen cards that
are finished and presses *Complete*. The head of the page has become the count and the verbs.
Two cards were assigned to somebody else and the organiser only contributes, so those two are
refused; the other twelve are done and the two refusals are named. `Escape` ends selecting. A
script does the same with one request of up to 500 operations.

## How to check

1. Selecting is a mode: entered by *Select* in the page menu, a long press on a touch screen, or
   Ctrl/Cmd-click on a row; while it is on, rows carry a checkbox, the page head shows the count and
   the verbs, and `Escape` ends it. Outside the mode no row draws a selection box.
2. The verbs are: complete, reopen, add a label, remove a label, assign to one person, move to a
   collection of the same hub, set the column, and move to the trash — the last after a
   confirmation.
3. Each entry is changed exactly as if it had been changed on its own: with its own permission
   check, its own history entry and its own event.
4. The answer lists a result per entry; a partly successful bulk is not reported as a failure, and
   every refusal names the entry and the reason.
5. More operations than the installation's limit (500 by default, published in the capabilities)
   are refused before anything is applied.
6. Through the API, `atomic` makes the bulk all or nothing: the first refusal rolls every change
   back.
7. A bulk may do nothing a person could not do one entry at a time — a contributor's bulk changes
   only the entries assigned to them.

## Where it ends

* No bulk due date, bulk custom field or bulk duplicate in the web app; the API's `UPDATE_ITEM`
  carries field changes for a script that needs them.
* No "select all 3 000 in the collection": a selection is what the person ticked, and a filter plus
  a script is the tool for everything.
* No undo of a bulk as a unit; each change is reversible on its own (reopen, remove label,
  restore from the trash).
