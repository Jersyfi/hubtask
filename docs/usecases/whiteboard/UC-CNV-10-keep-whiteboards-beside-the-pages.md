---
id: UC-CNV-10
title: Keep whiteboards beside the pages
context: whiteboard
actors: [PE-member, PE-person, PE-agent, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-05, P-08, P-19]
state: partial
tasks: []
checked_by: [apps/webapp/e2e/kompassmove.test.mjs, apps/webapp/src/lib/moving.test.ts, apps/webapp/src/lib/navigation.test.ts, apps/webapp/e2e/kompassscreens.test.mjs, core/domain/model/whiteboard/Board_test.go, core/application/service/whiteboard/Place_test.go, core/application/service/knowledge/Boards_test.go, test/integration/whiteboard_place_test.go]
---

# Keep whiteboards beside the pages

## Goal

A team files its whiteboards where they belong among its pages — the retro under *Team*, the story
map under *Product* — in the same tree, in the order it chooses.

## Story

After the sprint the team creates *Retro Sprint 41* from ＋ on the page *Team*. It stands under
*Team* between two pages. Later somebody drags the old *Story map* from the top level under
*Product*.

## How to check

1. A whiteboard carries its parent page (or none, at the top of the space) and its place among its
   siblings, pages and boards in one order; `GET /hubs/{hubId}/whiteboards` with `order=TREE`
   answers them in tree order.
2. A whiteboard is created under a page and after a sibling; `POST /whiteboards/{id}:move` places it
   under another page of the same hub or at the top, after a sibling page or board, with `WRITE_ITEMS`
   at the hub.
3. A page of another hub as the parent is refused `whiteboards.parent_other_space`; a whiteboard as
   the parent of a page or a board is refused `whiteboards.no_children`.
4. Trashing a page lifts the whiteboards directly under it to the page's parent at its place;
   nothing of a board is lost.
5. Boards that existed before stand at the top level after the pages, ordered by title.
6. In the sidebar's *Contents*, ＋ on a page offers *Whiteboard*, and a board is dragged before,
   after or under a page like a page, never into another board.
7. A board under a restricted page is still read by the hub's readers; whoever may not read the page
   sees the board under its nearest page they may read, or at the top.

## Where it ends

* A whiteboard has no children and moves to no other hub; copying stays export and import.
* A board's access stays the hub's; a page's restriction does not reach it.

## Today

* Check 7: met in part — a board under a page the reader does not see stands at the top of *Contents* (`apps/webapp/src/lib/navigation.test.ts`, `contentsTree`); the client does not know the hidden page's ancestors, so *its nearest page they may read* is not drawn.
