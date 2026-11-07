---
id: UC-WRK-39
title: Bring a collection to another hub without running into obstacles
context: work
actors: [PE-admin, PE-member, PE-agent, PE-scripter]
deployments: [D3, D4, D5, D6, D7]
serves: [P-03, P-05, P-06, P-08, P-11, P-12]
state: partial
tasks: []
checked_by: [core/application/service/work/PreviewContainerMove_test.go, cmd/hubctl/ContainerMovePreview_test.go, presentation/mcp/MovePreviewTool_test.go, apps/webapp/e2e/kompassmove.test.mjs, apps/webapp/src/lib/workspace/moveCheck.test.ts]
---

# Bring a collection to another hub without running into obstacles

## Goal

Before a collection moves to another hub, the person sees everything that would stop the move and
everything that changes with it, each obstacle with the one step that removes it, and only then
moves it.

## Story

Tom moves *Web app* from Product development to IT operations. The dialog says the name is free, but
18 entries use the type *Story* of Product development, and offers *Lift the type into the
workspace*. It warns that two people will no longer see the collection and that it takes the fields
and rules of IT operations. After he lifts the type, *Move* is enabled.

## How to check

1. `POST /containers/{containerId}:move-preview` with the body `:move` takes writes nothing and
   answers whether the move can be made, its obstacles and its consequences, each a message code
   with parameters.
2. The obstacles carry the codes the move itself refuses with — `containers.name_taken`,
   `entry_types.hub_bound` with the type and the number of entries, `workflows.hub_bound` with the
   workflow — and a destination where the caller lacks `STRUCTURE` is an obstacle.
3. The consequences name the accounts that lose read access and that the destination's structure
   applies.
4. A preview with no obstacle is followed by a move that succeeds when nothing changed in between;
   a preview of another workspace's collection answers as a missing one.
5. MCP and `hubctl` offer the same preview.
6. *Move to…* on a collection lists the hubs; those where the person lacks `STRUCTURE` are greyed
   with the reason.
7. The dialog shows each obstacle with its way out — a *New name* field, *Lift the type into the
   workspace* where the person may, choosing a workflow of the destination — and warnings for lost
   access and the structure taken over, in words, never a code.
8. *Move* is enabled only while no obstacle remains; on a phone the button names the destination.

## Where it ends

* No move of a hub, of a page or of a whiteboard to another hub.
* Lifting a type or a workflow is its own act (UC-FLW), offered here as a link, not done silently.
* Whatever the move does to hub-bound releases, SLA policies, calendars and portfolio projects is
  settled with the server in the concept's step 2 and then named as a consequence.

## Today

* Check 7: met in part — each obstacle shows with a way out and the warnings in words (`kompassmove.test.mjs`, `moveCheck.test.ts`); *Lift the type into the workspace* is not offered, because no operation lifts a hub's type into the workspace (`api/openapi.yaml` offers `copyEntryTypeHere` only into a hub) - the way out is *Change the entries' types* in the collection's settings; tracked in step 8 of the navigation concept (`docs/backlog/navigation/README.md`).
