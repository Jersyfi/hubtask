---
id: UC-WRK-03
title: Break a task down into smaller steps
context: work
actors: [PE-person, PE-member, PE-agent, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-08, P-11, P-12]
state: built
tasks: [B-03, B-05, B-08, F2-09, F10-12]
checked_by: [test/integration/capability_profile_test.go, test/integration/create_work_item_test.go, core/domain/model/work/CapabilityProfile_test.go, apps/webapp/e2e/entry.test.mjs]
---

# Break a task down into smaller steps

## Goal

A task too big to tick off in one go gets the steps under it — work packages, and activities
under those — and each level offers exactly what that level can carry, nothing the workspace
would refuse.

## Story

"Renovate the bathroom" is a task. On its page the person adds work packages ("Tiles",
"Plumbing") with *+ Work package*, and under "Tiles" activities ("Buy grout", "Book the tiler")
with *+ Activity*. The task's subtree shows how many of the first level are done. An activity is
small by design: one person responsible, a due date, a reminder, no labels and no notes.

## How to check

1. On a task's page, the add button offers the child types the server's capability profile
   permits under it — by default *Work package* under a task and *Activity* under a work package —
   and nothing under an activity.
2. Creating a type the profile does not permit under that parent is refused with
   `items.parent_type_invalid`; one deeper than the type's limit with `items.depth_exceeded`.
3. An entry's page shows only the fields and panels its type carries; for an activity there are no
   notes, labels, members, comments, cover, custom fields or recurrence.
4. Setting a field the type does not carry, through any channel, is refused with
   `capability_not_supported` naming the type and the capability — never silently ignored.
5. The subtree heading shows how many of its first-level children are completed, out of how many.
6. A child created under a collapsed row opens that row, so the new step is visible at once.
7. The web app takes the permitted child types and each type's fields from the capabilities the
   server publishes, not from a table of its own, so a changed profile changes the screen without
   a new release of the client.

## Where it ends

* No new entry types from the web app: a new level is a profile entry, decided by the installation
  ([domain model §2](../../architecture/domain-model.md)).
* A workspace narrowing the system's profile (for example: no activities) is documented there and
  has no writer yet; it would be an administration use case, not this one.
* No dependencies between steps ("B starts when A is done") — not planned.
* Completing a parent when its children are done is [UC-WRK-04](./UC-WRK-04-tick-work-off-and-take-it-back.md).
* An AI proposing the steps is the suggestion context's, and optional.
