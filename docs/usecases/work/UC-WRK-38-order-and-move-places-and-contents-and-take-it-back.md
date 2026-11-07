---
id: UC-WRK-38
title: Order and move places and contents in the sidebar, and take it back
context: work
actors: [PE-person, PE-member, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-05, P-11, P-13]
state: partial
tasks: []
checked_by: [apps/webapp/e2e/kompassmove.test.mjs, apps/webapp/src/lib/moving.test.ts, apps/webapp/src/lib/frame/toasts.test.ts, packages/design-system/test/navtree.test.js]
---

# Order and move places and contents in the sidebar, and take it back

## Goal

A person puts hubs, collections, queues, pages and whiteboards in order where they see them, by
pointer, keyboard or touch, sees before letting go what will happen, is told why something cannot
go there, and takes any move back.

## Story

Lena drags *Holiday rules* under *Team*; the folded page opens as she hovers, a line shows where it
lands, and *Undo* stays for eight seconds. She tries to drop a page onto a whiteboard: no line, and
the drag image says *A whiteboard has no subpages*. On the train she long-presses *Runbooks*,
chooses *Change order* and moves it with the grips.

## How to check

1. A drag in the sidebar starts only after 4 pixels of travel; a shorter press opens the row. The
   grip appears only on rows the person may order.
2. Over a row, the upper quarter places before, the lower quarter after, the middle inside (pages
   only); the line stands at the target level's indent; a folded page opens after 600 ms of hover;
   near the sidebar's edge it scrolls.
3. A drop that is not allowed draws no line, shows the *not allowed* pointer and one sentence on the
   drag image: into a whiteboard, under itself, into another part of the sidebar, or where the person
   may not write.
4. `Esc` during a drag cancels it and changes nothing.
5. After a drop the row settles and is highlighted, a toast offers *Undo* for 8 seconds, and
   `Cmd/Ctrl+Z` while it shows undoes the move; a screen reader hears the new position ("Position 2
   of 5 under Team").
6. By keyboard alone, `Alt+↑/↓` orders a focused row, `Alt+→/←` indents and outdents a page or a
   whiteboard, and `M` opens *Move to…*, whose places the person may not use are greyed with the
   reason.
7. Hubs are ordered only in the shared order (not while *Recently visited* or *A–Z* is chosen), by
   `STRUCTURE` at the workspace; collections and queues by `STRUCTURE` at the hub; pages and boards by
   `WRITE_ITEMS`; the new order is the same for every member after a reload.
8. Offline, a move shows at once with the mark *waiting*; a refusal by the server when it is back
   returns the row to where it was and says why.
9. On a touch screen a long press opens the context sheet with the row's own items; *Change order*
   opens reorder mode with grips at the end of each row and *Done*; no row drags outside reorder
   mode.

## Where it ends

* No move of a page or a whiteboard to another hub, no hub into another workspace, no dragging of
  several rows at once.
* A collection to another hub is [UC-WRK-39](./UC-WRK-39-bring-a-collection-to-another-hub-without-running-into-obstacles.md);
  an entry's own order and move stay [UC-WRK-06](./UC-WRK-06-put-work-in-order-and-move-it-where-it-belongs.md).

## Today

* Check 8: met in part — a move stands at once with *waiting* until the server answers and a refusal springs it back with its reason; offline the sidebar says the move needs a connection and that nothing changed (`kompassmove.test.mjs`). It is not kept to be sent later: the push frame carries no move of a page, a board or a container (`apps/webapp/src/lib/data/replica.ts` `mutationFor`), a change to the sync protocol named in the navigation concept's open points (`docs/backlog/navigation/README.md`).
