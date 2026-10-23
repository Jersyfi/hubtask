---
id: UC-WRK-28
title: Act on an entry the same way everywhere
context: work
actors: [PE-person, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-05, P-13]
state: partial
tasks: [UX-15, UX-16, UX-17]
checked_by: [apps/webapp/src/lib/entries/entryMenu.test.ts, apps/webapp/e2e/entrymenu.test.mjs, apps/webapp/e2e/entrybyrole.test.mjs, apps/webapp/e2e/undo.test.mjs, apps/webapp/src/lib/entries/undo.test.ts, test/integration/restore_label_test.go, test/integration/bulk_restore_test.go, apps/webapp/e2e/layoutmemory.test.mjs, apps/webapp/src/lib/entries/layoutmemory.test.ts, apps/webapp/e2e/today.test.mjs]
---

# Act on an entry the same way everywhere

## Goal

Whether an entry shows up in a list, on a board, on a timeline, in search results, on *Today* or open on its own page, the same actions are in the same menu, and a mistake can be taken back.

## Story

On the board the person right-clicks a card and moves it to another collection — the same menu they know from the list. They move a task to the trash by mistake; the message offers *Undo*.

## How to check

1. Every place that shows an entry offers the same menu by right-click, by its *⋯* button and by long-press on touch: open, tick off, dates, person, labels, move (up, down, to the top, to the bottom, inside, out, to another collection), duplicate, share, copy link, archive, move to the trash; only actions the person may perform and the entry's type supports are listed.
2. The entry's own page carries the same menu in its head.
3. Moving to the trash, archiving, removing a label and deleting a label from a collection take effect at once and offer *Undo* for at least eight seconds; actions that cannot be undone ask first.
4. Selecting several entries works by `Ctrl`/`⌘`-click, and on touch by a long press and the entry
   menu's *Select*, in the list and on the board.
5. On wide screens, opening an entry from the list, the board or the timeline shows it in the detail pane.
6. The layout chosen for a collection (list, board, timeline) is kept for the person on that device.

## Where it ends

* What the actions do is their own use cases ([UC-WRK-06](./UC-WRK-06-put-work-in-order-and-move-it-where-it-belongs.md), [UC-LIF-01](../lifecycle/UC-LIF-01-get-something-back-from-the-trash.md) and others); this one is where they are.

## Today

* Check 2: not met — not built yet, tracked in milestone UX (UX-17).
