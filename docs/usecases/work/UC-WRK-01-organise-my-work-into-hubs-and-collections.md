---
id: UC-WRK-01
title: Organise my work into hubs and collections
context: work
actors: [PE-person, PE-owner, PE-admin, PE-member, PE-scripter, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-05, P-08, P-10, P-12]
state: partial
tasks: [A-07, B-06, F2-04, F2-08, F10-01, F10-03]
checked_by: [test/integration/create_container_test.go, test/integration/container_test.go, test/integration/container_lifecycle_test.go, apps/webapp/e2e/container.test.mjs, apps/webapp/e2e/archive.test.mjs]
---

# Organise my work into hubs and collections

## Goal

A person shapes the workspace after their own life or team: a hub per area ("Home", "Club",
"Customer X"), collections inside it ("Groceries", "Renovation"), named, ordered and moved as the
areas change — and nobody who may not shape the workspace is offered to.

## Story

In `D1` the person opens an empty workspace and the overview offers one thing: *Create hub*. They
name it, then add collections from the hub's page. Later they rename a collection, move it to
another hub, push a hub up the list, or archive a collection they no longer use and find it again
under *Archive*.

In `D2`–`D4` shaping the workspace is the job of whoever holds the owner's or an administrator's
role on the workspace; somebody administering one hub can add collections to that hub only. A
member or a child works *inside* the collections they are given.

## How to check

1. A person with the owner's or an administrator's role on the workspace creates a hub with a
   name and an optional description; it appears in the navigation at once, after the existing hubs.
2. A person with an administrator's role on one hub creates a collection in that hub, and is
   refused when creating a hub.
3. A second hub, or a second collection in the same hub, with the same name — compared ignoring
   case and after Unicode normalisation — is refused with `containers.name_taken` naming the name.
4. A collection is renamed from its page menu, moved to another hub through the move dialog (which
   offers only hubs that are not archived), and hubs and collections are moved up and down among
   their siblings; the new order is the same on every device.
5. An archived hub or collection leaves the navigation, is listed under *Archive* and can be
   brought back from there; while archived, nothing can be added to it and the page says why.
6. A person without the right to change the workspace's shape (a member, contributor, viewer or
   guest) is **not shown** *Create hub*, *Create collection*, *Rename* or *Move*.
7. A person who can see no hub because their role reaches none is told so, and is not offered to
   create one.
8. Everything above is also possible through the API, `hubctl` and MCP, with the same refusals.

## Where it ends

* An icon or a colour per hub or collection is not asked for yet (the create dialog carries name
  and description only).
* Deleting a hub or a collection — to the trash, restoring it, emptying the trash — belongs to the
  lifecycle use cases, not here.
* No nesting deeper than hub › collection: a collection holds entries, not collections
  ([domain model §3.3](../../architecture/domain-model.md)).
* No drag and drop in the navigation tree is required; *Move up*/*Move down* is the reorder.

## Today

* **Check 6 fails.** *Create hub* is drawn for every signed-in person — in the overview's head,
  the navigation's group header and its empty state
  (`apps/webapp/src/views/HomeView.svelte:84`, `apps/webapp/src/lib/frame/WorkspaceNav.svelte:210-224`),
  and *Create collection* on every hub page (`apps/webapp/src/views/ContainerView.svelte:643-650`).
  Only the server's refusal stops a member.
* **Check 7 fails.** A person whose memberships reach no hub sees the "no hubs yet, create one"
  empty state, because the client cannot tell an empty workspace from one narrowed to nothing
  (`apps/webapp/src/lib/data/containers.svelte.ts:78`).
