---
id: UC-WRK-10
title: Hand work to a person
context: work
actors: [PE-member, PE-admin, PE-person, PE-child, PE-agent, PE-integrator]
deployments: [D2, D3, D4, D5, D6, D7]
serves: [P-05, P-08, P-11, P-12]
state: built
tasks: [C-01, F3-07, F10-13]
checked_by: [test/integration/assignment_test.go, core/application/service/work/AssignWorkItem_test.go, core/application/service/work/AddMember_test.go, core/application/service/work/CreateWorkItemAssignment_test.go, apps/webapp/e2e/entry.test.mjs]
---

# Hand work to a person

## Goal

Every entry can answer two questions — *who is responsible* and *who else is on it* — and the
person handed something learns about it, while nobody can be handed work in a place they cannot
see.

## Story

A parent opens "Mow the lawn" and picks the teenager under *Responsible*; under *Also on it* they
add the other parent. The teenager is told by mail (if the household has one, and unless they
switched that kind of message off). Later the task goes to somebody else: one step in the
history, "handed from A to B". An activity has only the first question; it is one person's to do.

## How to check

1. The entry shows *Responsible* (one person or nobody) and *Also on it* (several people) as two
   separate questions; an activity shows only the first.
2. The people offered are those who can see the entry — memberships on the workspace, the hub,
   the collection or the entry itself, groups expanded to their people.
3. Handing an entry to somebody who cannot see it is refused with `items.account_without_access`,
   which says to give them access first.
4. Changing the responsible person writes one `item.assigned` step with both sides; clearing it
   writes `item.unassigned`; adding and removing others writes `item.member_added` and
   `item.member_removed`.
5. The newly responsible person is notified in the `ASSIGNMENT` category and a newly added member
   in the `MEMBERSHIP` category, unless their own preference switched that category off.
6. A person with the contributor's role who creates an entry is its responsible person
   automatically; naming somebody else, or asking the collection's policy to hand it out, is
   refused.
7. A contributor can change only the entries they are responsible for; a viewer or a guest can
   hand nothing to anybody.
8. Assigning is possible through the API, MCP, `hubctl`, a bulk and an automation action, with the
   same refusals.

## Where it ends

* No more than one responsible person per entry — the second question exists for the rest.
* No workload planning or capacity view.
* Letting the collection pick the person is [UC-WRK-11](./UC-WRK-11-let-a-collection-hand-out-new-work.md).
* Giving somebody access to a hub, a collection or an entry is [UC-WRK-21](./UC-WRK-21-share-a-hub-a-collection-or-one-entry.md).
* Notifications other than by mail belong to the notification context; a household without a mail
  server learns of an assignment by looking.
