---
id: UC-LIF-01
title: Get something I deleted back from the trash
context: lifecycle
actors: [PE-person, PE-member, PE-owner, PE-scripter, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-08, P-11, P-12]
state: partial
tasks: [B-10, F2-14]
checked_by: [core/application/service/work/TrashWorkItem_test.go, core/application/service/work/TrashContainer_test.go, core/application/service/work/ReadTrash_test.go, test/integration/trash_test.go, cmd/hubctl/Trash_test.go]
---

# Get something I deleted back from the trash

## Goal

Deleting is never the end: whatever a person deletes waits in the trash, whole, for the trash
period (30 days by default), and they can put it back exactly as it was with one action.

## Story

A person deletes a task by mistake — with its work packages and activities. It disappears from the
list. In the trash they find it with when it was deleted, by whom, and how many days are left. They
choose *Restore*, and the task is back where it was, with everything that went with it; a task that
was archived when it was deleted comes back archived. Deleting a whole collection works the same way
and comes back as one piece. A script does the same with `hubctl item rm` and `hubctl trash restore`,
an agent through its MCP tools.

## How to check

1. Deleting a task, a collection or a hub removes it and everything under it from every list and
   puts it in the trash as one deletion.
2. The trash lists each deletion with its moment, who deleted it (*you*, a named person, or an
   automation) and the days left in the trash period.
3. Restoring a deletion brings back everything that went with it, in its old place and state —
   nothing more and nothing less.
4. A person who may edit a task may delete and restore it; only the owner may delete a hub or a
   collection, and nobody else sees that control.
5. A deleted task cannot be changed; a change is refused with a message saying it is in the trash.
6. Deleting and restoring are possible in the web app, through the API, with `hubctl` and through
   MCP, for tasks and for hubs and collections alike.
7. Each deletion and restore writes an entry in the trail (`item.trashed`, `item.restored`,
   `container.deleted`, `container.restored`).

## Where it ends

* No version history: the trash brings back the deleted thing, not an earlier edit of it.
* Something removed for good — by emptying the trash or after the period — does not come back from
  the trash; a restore from a backup is the only way (backup context).
* A device that was offline does not bring a deleted thing back; that is the sync context's
  promise.

## Today

* Check 6: not met for `hubctl` — it has no command to delete a hub or a collection.
