---
id: UC-ID-16
title: Keep a hub private, even from another administrator
context: identity
actors: [PE-member, PE-child, PE-owner, PE-admin]
deployments: [D2, D3, D4, D5, D6]
serves: [P-01, P-04, P-05, P-06, P-10]
state: specified
tasks: [PH-04, PH-05]
checked_by: []
---

# Keep a hub private, even from another administrator

## Goal

In a family or a small team, a person keeps a hub — a diary, a present list, a private project —
that nobody else can open, including the adults and administrators who run the workspace; and if
the workspace owner ever has to open it, the people it belongs to know at once
([ADR-0073](../../adr/ADR-0073-private-hubs.md)).

## Story

Maria creates *Presents* and marks it private. Her partner is an administrator of the family
workspace too; he sees neither its name nor its contents — only that Maria has a private hub and how
much it holds. Lena, twelve and a plain member, has her own private hub for her diary. When Maria
falls ill and her partner, the workspace owner, must reach something in *Presents*, he uses the
emergency access: a fresh proof, a reason, one hour — and Maria is told at once. In a company the IT
administrator decided private hubs are not allowed; nobody there can make one.

## How to check

1. A person may create a private hub for themselves wherever the workspace allows private hubs,
   whatever their role; a person who may create hubs may mark one they own private.
2. A private hub's name and contents are unreachable for everybody who is not a member of that hub
   — including the workspace's owners and administrators — in navigation, the hub list, search, the
   overview, the calendar feed, saved views, automation run by others and AI run by others; an
   attempt answers "not found".
3. Administrators see, per private hub, whose it is and how much it holds, without its name.
4. The owner — only the owner — can open a private hub through the emergency access: a fresh proof,
   a stated reason, one hour, not renewable. Every member of the hub is notified at once with the
   time and the reason, and the access is in the workspace's trail.
5. Backups and the workspace export contain private hubs, marked as private; a restore keeps them
   private. Retention rules and legal holds apply to them.
6. When the last member leaves, the hub goes to the trash with the ordinary grace period and the
   owner is told; nothing is deleted silently.
7. *Private hubs allowed* is a workspace setting, default on, with the installation's and the plan's
   default and lock shown beside it; switching it off makes no existing private hub visible and
   stops new ones.
8. In a workspace with one person (D1) nothing of this appears.

## Where it ends

* Not encryption: whoever runs the installation's database and backups can technically read it
  ([NG-e2e-encryption](../../vision/non-goals.md)).
* No secret access: the emergency access always notifies; there is no silent variant for anyone.
* No private collections or entries inside a shared hub; privacy is per hub.

## Today

* Check 1: not met — there are no private hubs; rights only add up in the authorisation model, tracked in #1089.
* Check 2: not met — nothing hides a hub from the workspace's owners and administrators, tracked in #1089.
* Check 3: not met — there is no per-hub overview without the name for administrators, tracked in #1089.
* Check 4: not met — there is no emergency access, tracked in #1090.
* Check 5: not met — backups, the export and a restore know no private hubs, tracked in #1089.
* Check 6: not met — nothing handles the last member leaving a private hub, tracked in #1089.
* Check 7: not met — there is no *Private hubs allowed* setting, tracked in #1090.
