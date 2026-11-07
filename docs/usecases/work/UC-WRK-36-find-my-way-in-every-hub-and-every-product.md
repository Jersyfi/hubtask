---
id: UC-WRK-36
title: Find my way in every hub and every product
context: work
actors: [PE-person, PE-member, PE-handler, PE-planner, PE-child]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-05, P-10, P-12, P-13, P-17]
state: partial
tasks: []
checked_by: [apps/webapp/src/lib/navigation.test.ts, apps/webapp/src/lib/workspace/placeViews.test.ts, apps/webapp/e2e/kompass.test.mjs, apps/webapp/e2e/kompassscreens.test.mjs, apps/webapp/src/lib/routes.test.ts, apps/webapp/src/lib/product/home.test.ts, apps/webapp/src/lib/portal/portal.test.ts, apps/webapp/src/lib/phone.test.ts, apps/webapp/e2e/kompassphone.test.mjs]
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
18. Creating a collection in a hub (*New ▾ → Collection*), a member sees the kinds and *Desk*.
    *Desk* makes a collection that starts from *Tasks* and offers the hub's request types open to
    the portal, so it acts as a desk; in a hub without such a type, *Desk* says *Needs a portal
    type in the hub* and cannot be chosen.

## Where it ends

* Moving and ordering are [UC-WRK-38](./UC-WRK-38-order-and-move-places-and-contents-and-take-it-back.md),
  stars [UC-WRK-37](./UC-WRK-37-star-what-i-come-back-to.md).
* Hub tools are not ordered or hidden by a person; a tool is switched off at the hub (UC-INS-21).
* No folders of hubs, no folders in *Contents*.
* Switching workspaces stays in the account menu (UC-ID-25).

## Today

* Check 1: met — `kompass.test.mjs` (the workspace level from the top; no *Starred* before a star, no *Administration* without the right); the order of the rows `navigation.test.ts` (`forYou`, `foot`). *Recent* is the device's last five places (`recents.ts` `MAX`), not twenty.
* Check 2: met — `kompass.test.mjs` (a hub, a collection, a tool; focus on *‹ All hubs*); `navigation.test.ts` (`levelOf` for every route of a hub). A page, a board and an entry show their hub's level once their screen has named it (`page.locate`). The hub's ⋯ is the page menu's (`placeMenu`).
* Check 3: met — `navigation.test.ts` (`hubTools`: the fixed order, each module, *Queues* only with a desk) and `kompass.test.mjs` (the hub's screen holds no tool as a tab).
* Check 4: met — `kompass.test.mjs` (a starred queue first, the breaching count on the danger surface with its words as the figure's label); `navigation.test.ts` (`queueRows`).
* Check 5: met — `kompass.test.mjs` (one tree, the template left out, a board without a twist; the mode written to `nav_contents_mode`); `navigation.test.ts` (`contentsTree`: the shared key, keyless boards A–Z, the modes). The filter from 30 is `NavGroup`'s (`packages/design-system/test/navtree.test.js`).
* Check 6: met — `kompass.test.mjs` (the desk mark, the sprint mark, the figures); *n more* from 15 and the filter from 20 are `NavGroup`'s bounds, set from `sidebar.ts`.
* Check 7: met — `kompass.test.mjs` (Plan to Plan; a hub without the tool opens its overview; the search); `navigation.test.ts` (`switchPath`).
* Check 8: met — `kompass.test.mjs` (the kept width, a double-click to 264, the drag bounded at 400, `Ctrl+[`, the peek opening and closing).
* Check 9: met — `kompassscreens.test.mjs` (Plan's, Goals' and Inventory's one bar, *Manage* a menu among the parts; a collection's bar, its former *Plan* its *Timeline*), `placeViews.test.ts`; the product homes' bars `home.test.ts`.
* Check 10: met in part — `kompassscreens.test.mjs` (no tab, the tools' lines, the collection cards with their figures); *Recently changed* is `HubActivity`'s, not walked.
* Check 11: met in part — `home.test.ts` (`homeTabs`: the eight homes' tabs, *For you* first, *Settings* last, *Roadmaps* on Plans); whether *Settings* is left out for a reader who may not read them is not walked.
* Check 12: met — `routes.test.ts` (every former address and where it goes) and `kompassscreens.test.mjs` (each address walked, the desk's to the first queue, the pages' to the home page).
* Check 13: met — `kompassphone.test.mjs` (the bar's five tabs; *Browse* the workspace level, a hub's three parts and *Queues* as steps; another tab and back returns to the step left, a second tap to the root); `phone.test.ts` (`phoneTabs`, `tabOf`, `TabStacks`).
* Check 14: met — `kompassphone.test.mjs` (*‹ Hubs*, the title, ☆ and ⋯ on a hub's step, *‹ Product development* on a collection; a content row's name links the page and its › descends; the view chip's sheet lists the desk bar's views in their order); `phone.test.ts` (`ascentOf`, `starOf`).
* Check 15: met — `kompassphone.test.mjs` (*IT operations, level 2* and *Queues, level 3* announced; every control of the bar, the bottom bar, each Browse step and the sheets 44 pixels or more; *New*, the account and the views' sheets with their heading and *Close*).
* Check 16: met — `kompassphone.test.mjs` (at 768 no bottom bar, ☰ opens the drawer with the workspace level and, in a hub, its level; a Browse address opens the hub).
* Check 17: met — `kompass.test.mjs` (no tool, no *Contents*) and `navigation.test.ts` (`hubTools` with nothing on).
* Check 18: met — `kompassscreens.test.mjs` (*Desk* chosen sends *Tasks* and the hub's portal type as the collection's request types; without a portal type the card says so and its *Choose* is disabled); `portal.test.ts` (`portalTypesOf`).
