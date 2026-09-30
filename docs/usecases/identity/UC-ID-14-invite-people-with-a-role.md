---
id: UC-ID-14
title: Invite people and give them a role
context: identity
actors: [PE-owner, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6]
serves: [P-05, P-09, P-10, P-12]
state: partial
tasks: [H-01, B-02]
checked_by: [core/application/service/identity/InviteAccount_test.go]
---

# Invite people and give them a role

## Goal

An owner or administrator brings a person in — a family member, a colleague, a client — with the
role and the part of the workspace they should have, and the person receives the invitation even
where the installation has no mail server.

## Story

*People → Invite*: address, role (Administrator, Member, Contributor, Viewer, Guest) and where —
the whole workspace, one hub, one collection or one task. The invitation goes out by mail. Where
the installation has no mail configured (a home server), the dialog says so and offers *Copy
invitation link* instead, to hand over by any messenger.

## How to check

1. The dialog offers only roles the inviting person may give; an administrator is not offered
   *Owner*.
2. The scope can be the workspace, a hub, a collection or a single task; the invited person later
   sees exactly that part.
3. With mail configured, the invitation arrives by mail and is valid for the stated time.
4. The trail records the invitation with the role and scope, without the address.
5. Without mail configured, the dialog does not pretend to send: it offers a link to copy, shown
   once, which works exactly like the mailed one.
6. Inviting an address that already has an account here says so, rather than sending a second
   invitation.

## Where it ends

* No bulk import of people from a file; a directory provider (UC-ID-08) covers the many-people case.
* No accounts without an address — that is UC-ID-20.

## Today

* **Check 5 fails:** the invitation is only ever mailed; a household without a mail server cannot
  invite anybody except by provisioning through the control plane.
