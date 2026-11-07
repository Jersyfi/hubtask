---
id: UC-WRK-37
title: Star what I come back to
context: work
actors: [PE-person, PE-member, PE-handler, PE-planner, PE-agent, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-08, P-10, P-13]
state: partial
tasks: []
checked_by: [apps/webapp/e2e/kompass.test.mjs, apps/webapp/e2e/kompassmove.test.mjs, apps/webapp/src/lib/data/stars.test.ts, core/domain/model/identity/Star_test.go, core/application/service/identity/Stars_test.go, test/integration/star_repository_test.go]
---

# Star what I come back to

## Goal

A person keeps the few places they return to every day — a collection, a queue, a page, a hub — in
one list of their own, in the order they choose, reachable from anywhere and on every device.

## Story

Jonas stars *About to breach* in IT operations, the collection *Web app* and the onboarding
handbook. They stand under *Starred* on the workspace level; he drags the queue to the top. At home
on his phone they are the same three, in the same order.

## How to check

1. `GET /accounts/{accountId}/stars` answers the caller's own stars in their order; another
   account's stars are refused as another account's preferences are.
2. `PUT /accounts/{accountId}/stars` replaces the whole ordered list of at most 100 stars of the
   kinds hub, collection, view, page, whiteboard, dashboard and portfolio; a stale `If-Match` is
   refused as `version_conflict`, more than 100 or an unknown kind as invalid.
3. A star of something the caller may not read — a private hub's page, a collection after the
   membership ended — is not answered, and the next write drops it; another workspace's stars are
   never read (cross-tenant test).
4. The trail records that the stars changed and how many there are, never which.
5. ☆ in a place's head or `S` stars and unstars it; the place appears under *Starred* on the
   workspace level at once, and *Starred* appears only once a star exists.
6. Dragging a row under *Starred* reorders the stars, personally and without asking; the order is
   the same on another device after a reload.

## Where it ends

* No shared stars, no folders of stars; a team's shared order is the hubs' own order.
* MCP and `hubctl` read and write stars through the same operations (P-08); no automation trigger.

## Today

* Check 5: met in part — ☆ in a hub's head stars and unstars it and the hub stands under *Starred* at once; *Starred* appears only with a star (`apps/webapp/e2e/kompass.test.mjs`, `apps/webapp/src/lib/data/stars.test.ts`). `S` and the ☆ in the heads of a collection, a queue, a page and a board come with step 5's screens.
