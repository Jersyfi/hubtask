---
id: UC-WRK-45
title: Overview a queue and a list
context: work
actors: [PE-member, PE-person, PE-handler]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-12, P-13]
state: verified
tasks: []
checked_by: [apps/webapp/e2e/calmlist.test.mjs, apps/webapp/e2e/listarrangement.test.mjs, apps/webapp/e2e/loadmore.test.mjs, apps/webapp/e2e/productsband.test.mjs, apps/webapp/src/lib/entries/statechip.test.ts, apps/webapp/src/lib/entries/listview.test.ts]
---

# Overview a queue and a list

## Goal

A person reads a collection's list or a desk's queue at a glance as one calm table - the columns
its kind needs, in groups that say how many they hold - and acts on a row from where it stands,
the same with a mouse, the keyboard or a finger.

## Story

Amelia opens *Team Atlas*. The list is one region: the column heads in small capitals, the entries
ruled apart by hairlines, each with its state control, title, due date, person, labels and parts.
She groups it by column: *To do 3*, *Doing 1*, each ending in *+ Entry in "To do"*, and folds
*Doing*. Pointing at a row shows *Open* and ⋯. With the keyboard she walks the rows with ↓, picks
two with Space and opens the third with Enter. Above the list the tools say their words -
*Filter*, *Fields*, *Group*, *Sort*, *Views*, *Comfortable · Compact*, *Select entries*.

Thomas works the IT desk's queue: key, request, type, requester, priority, the SLA as a bar with
its words, the assignee, grouped by status with what breaks first on top. On his phone each request
is its title with the key, status and priority under it and the SLA bar; a swipe to the right
completes it, to the left opens its menu, which can do the same.

In `D1` a list holds a handful of entries in one group; in `D4`–`D7` the groups are long and the
foot loads more.

## How to check

1. At 1280×900 a member opens a collection's list: one region holds the column heads and the
   entries, ruled apart by hairlines rather than separate cards; a list's columns are the state
   control, the title, the due date, the person, the labels and the parts; a desk's collection
   shows the queue's columns of check 9.
2. Grouped by column, each group has a head with its name and count that folds and unfolds it, and
   ends in *+ Entry in "<column>"*, which opens capture and places the new entry in that column.
3. Pointing at a row shows one act, *Open*, and ⋯; the ⋯ menu offers *Open* too, and without a
   pointer the act and ⋯ stay reachable by the keyboard.
4. The list is comfortable or compact by the person's choice, and this device keeps the choice.
5. On the list ↓ and ↑ move from one row's title to the next, Enter opens the entry and Space picks
   it for the bulk bar; picking a row never completes it.
6. At 1280×900 the list's tools stand above it in words - the query's pieces, *View* (filter,
   fields, grouping, order and the saved views in one panel), the density, *Select entries* - not
   as bare marks.
7. At 390×844 a row swiped to the right is completed (at L1 and L2 carried to the next column or
   along the first step), swiped to the left opens its menu; the menu offers the same act, and a
   line under the list says so.
8. A list longer than one page ends in a control that loads more at its foot; there are no page
   numbers.
9. A handler opens a queue: its table shows key, request, type, requester, priority, the SLA as a
   bar with words and the assignee, grouped by status with each group's count and *+ Request in
   <desk>*; on a phone each request is its title with key, status and priority under it and the
   SLA bar.

## Where it ends

* Which columns stand, what is filtered, how it is ordered and grouped and saving it as a view is
  one panel, *Ansicht* ([UC-WRK-17](./UC-WRK-17-filter-sort-and-save-a-view-of-my-work.md)).
* The board's cards and columns are [UC-WRK-05](./UC-WRK-05-run-a-collection-as-a-board.md).
* Ranking and moving entries is [UC-WRK-06](./UC-WRK-06-put-work-in-order-and-move-it-where-it-belongs.md).
* *+ Entry* stands only under a group capture can place an entry in (a column); under a group by
  person, due date or category the one add at the list's foot remains.
