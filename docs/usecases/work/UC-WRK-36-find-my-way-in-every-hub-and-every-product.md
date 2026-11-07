---
id: UC-WRK-36
title: Find my way in every hub and every product
context: work
actors: [PE-person, PE-member, PE-handler, PE-planner, PE-child]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-05, P-10, P-12, P-13, P-17]
state: specified
tasks: []
checked_by: []
---

# Find my way in every hub and every product

## Goal

Wherever a person stands, the sidebar answers where they are and what is here in one shape: the
workspace with its products and hubs, or one hub with its tools, its collections and its contents.
Every product has one place in a hub and one home across the hubs, and a household that switches
nothing on sees only its lists.

## Story

Mara opens *IT operations*. The sidebar becomes the hub: *Queues* with *About to breach · 1* in red,
*Inventory* with *6 !*, then the collections, then the pages and the network plan in one tree. She
opens the queue, then *‹ All hubs*, and is back among the products and hubs. On her phone the same
hub is one step of a stack under *Browse*. Her neighbour's household hub shows the overview and
three lists, nothing else.

## How to check

1. On *Today*, a member sees in the sidebar, from the top: *For you* (*Today*, *Inbox*, *Waiting for
   your yes* where it applies), *Recent*, *Starred* (only once something is starred), *Products*
   (only modules on), *Hubs*, and at the foot *Administration* (only with `STRUCTURE` or
   `READ_CONFIGURATION`), *Archive*, *Trash*.
2. Opening a hub, a collection, a page, a whiteboard, a queue or a hub tool shows that hub's level:
   *‹ All hubs*, the hub's mark and name with ▾, ☆ and ⋯, then *Overview* and the hub tools, then
   *Collections*, then *Contents*; *For you* and the foot stay; focus lands on *‹ All hubs*.
3. The hub tools appear in the fixed order *Queues*, *Plan*, *Inventory*, *Time*, *Goals*, each only
   where its module is on at that hub; *Queues* only where the hub also has a desk. No hub tool is
   a tab and no product appears twice on the screen.
4. *Queues* expands to the hub's queues, starred ones first, each with its count; a queue holding an
   entry about to breach or breached shows its count on the danger surface with words, never colour
   alone.
5. *Contents* lists the hub's pages and whiteboards in one tree; a whiteboard has no twist; from 30
   entries a filter with *All · Pages · Whiteboards* shows matches with their ancestors; the
   section's ⋯ switches between *Tree*, *Recently changed* and *A–Z*, kept for the person on
   another device.
6. A collection's row carries its kind's mark (desk, runs sprints) and its figure; from 15
   collections the rest are behind *n more*, from 20 a filter appears.
7. ▾ beside the hub's name opens the quick switch (search, starred and recently visited hubs, *All
   hubs*); choosing a hub that has the current tool opens that tool there, otherwise its overview.
8. The sidebar is dragged between 200 and 400 pixels, a double-click returns it to 264, and the width
   is kept on this device; `Ctrl+[` folds it to the rail, hovering the rail opens the peek and
   leaving it closes the peek.
9. A collection, a hub tool and a product home draw one view bar under the head and never a second
   strip; a collection's former *Plan* is its *Timeline* view; the assets' *Manage* is a menu.
10. The hub's overview carries no tabs: its tools' lines, the collection cards with their figures,
    *Recently changed*.
11. Each product home shows its product across every hub with the tabs of its kind — *For you* first
    where it has personal work, *Settings* last where the reader may read them — and the Plans home
    has *Roadmaps*.
12. An address of the former hub tabs (`/hubs/:id/desk`, `/hubs/:id/pages`, `/hubs/:id/whiteboards`,
    `/hubs/:id/assets`, `/hubs/:id/actuals`, `/goals?hub=`) opens the place that took its role.
13. On a phone (390 pixels) the bottom bar is *Today · Browse · ＋ · Inbox · Search*; *Browse* shows
    the workspace level, a hub its three parts as a stack step, *Queues* a step of its own; each tab
    keeps its own stack and a second tap returns to its root.
14. On a phone a deeper step's bar shows *‹ the level above*, the title, ☆ and ⋯; a content row opens
    on its name and descends by its separate ›; a collection's view is a chip whose sheet lists the
    same views in the same order.
15. On a phone every stack change is announced with the place and its level, every target is at
    least 44 pixels, and every sheet has a heading and *Close*.
16. Between 600 and 904 pixels the sidebar is a drawer with both levels and no bottom bar.
17. A workspace in which no module is on shows no hub tool and no *Contents* in any hub (D1).

## Where it ends

* Moving and ordering are [UC-WRK-38](./UC-WRK-38-order-and-move-places-and-contents-and-take-it-back.md),
  stars [UC-WRK-37](./UC-WRK-37-star-what-i-come-back-to.md).
* Hub tools are not ordered or hidden by a person; a tool is switched off at the hub (UC-INS-21).
* No folders of hubs, no folders in *Contents*.
* Switching workspaces stays in the account menu (UC-ID-25).

## Today

* Check 1: not met — built in step 4 of the navigation concept (`docs/backlog/navigation/README.md`).
* Check 2: not met — built in step 4 of the navigation concept (`docs/backlog/navigation/README.md`).
* Check 3: not met — built in step 4 of the navigation concept (`docs/backlog/navigation/README.md`).
* Check 4: not met — built in step 4 of the navigation concept (`docs/backlog/navigation/README.md`); the queues' counts it shows are read by `GET /hubs/{hubId}/queues`, built in step 2.
* Check 5: not met — built in step 4 of the navigation concept (`docs/backlog/navigation/README.md`).
* Check 6: not met — built in step 4 of the navigation concept (`docs/backlog/navigation/README.md`).
* Check 7: not met — built in step 4 of the navigation concept (`docs/backlog/navigation/README.md`).
* Check 8: not met — built in step 4 of the navigation concept (`docs/backlog/navigation/README.md`).
* Check 9: not met — built in step 5 of the navigation concept (`docs/backlog/navigation/README.md`).
* Check 10: not met — built in step 5 of the navigation concept (`docs/backlog/navigation/README.md`).
* Check 11: not met — built in step 5 of the navigation concept (`docs/backlog/navigation/README.md`).
* Check 12: not met — built in step 5 of the navigation concept (`docs/backlog/navigation/README.md`).
* Check 13: not met — built in step 6 of the navigation concept (`docs/backlog/navigation/README.md`).
* Check 14: not met — built in step 6 of the navigation concept (`docs/backlog/navigation/README.md`).
* Check 15: not met — built in step 6 of the navigation concept (`docs/backlog/navigation/README.md`).
* Check 16: not met — built in step 6 of the navigation concept (`docs/backlog/navigation/README.md`).
* Check 17: not met — built in step 4 of the navigation concept (`docs/backlog/navigation/README.md`).
