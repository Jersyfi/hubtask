---
id: UC-WRK-21
title: Share a hub, a collection or one entry with somebody
context: work
actors: [PE-owner, PE-admin, PE-member, PE-guest, PE-child, PE-scripter]
deployments: [D2, D3, D4, D5, D6, D7]
serves: [P-01, P-05, P-07, P-08, P-10]
state: partial
tasks: [B-02, F3-01, F3-07, F4-09]
checked_by: [test/integration/membership_test.go, test/integration/membership_list_test.go, core/domain/service/Authorization_test.go, core/domain/service/ItemAccess_test.go]
---

# Share a hub, a collection or one entry with somebody

## Goal

From the work itself, the person who runs it lets somebody in at exactly the width that is needed —
the child on their chores hub, the tiler on the one task about tiles, a volunteer on the fair's
collection — with a role that says what they may do there, and sees at a glance who already has
access and from where.

## Story

On the hub "Lena's chores" a parent opens *Members*, picks Lena and *Contributor*. On the task
"Tile the bathroom" they choose *Share* and give the tiler *Guest*: he sees that one task, can
comment, and nothing else of the family's workspace shows. The dialog lists everybody who reaches
the entry — those granted here, and, marked as inherited, those granted on the collection, the
hub or the workspace, which can only be revoked where they were granted. Revoking asks first.

## How to check

1. The members dialog is reachable from a hub, a collection and an entry (*Share*), and lists
   everyone who reaches that place: granted here, or inherited from above with where it comes from;
   a group is shown by its name and its people.
2. A person with the right to manage members grants any of the roles the server publishes, at this
   place, and revokes a grant made here after a confirmation; inherited rows cannot be revoked here.
3. Somebody granted a role on one hub sees that hub and nothing else in the navigation, the search
   and the overview; somebody granted a role on one entry sees that entry only, and a guest there
   can read and comment and nothing more.
4. A person **not yet on the path** — invited to the workspace without a role, or holding a role
   only on another hub — can be chosen in the dialog and granted a role here.
5. A person can be invited from the web app **with a role on one hub or one entry only**, without
   being given a role on the whole workspace first.
6. Granting the owner's role asks the granting person to prove it is them again.
7. A person without the right to manage members sees who has access but is not offered to grant
   or revoke.
8. Every grant and revocation is in the audit trail, with who granted what to whom at which place.

## Where it ends

* Rights only add up: a grant on a hub never takes away what a role on the workspace gives. A hub
  one adult keeps even from another administrator is a separate use case (UC-ID-16) and is not
  built; in `D2` today, every adult who holds a workspace role reads every hub.
* Inviting somebody and how they sign in belong to the identity context (UC-ID-14, UC-ID-15).
* No public or anonymous link to an entry.
* No time-limited shares.

## Today

* Check 1: not met for groups — a group holding a role is listed as "a group", without its name.
* Check 4: not met in the web app — the members dialog offers only people who already hold a membership along the path, tracked in #1079.
* Check 5: not met in the web app — the invitation always grants its role on the whole workspace, tracked in #1079.
* Check 7: not met — *Share* is on every entry's menu for every reader, and the members dialog draws its grant and revoke controls without asking the role; only the server refuses them.
