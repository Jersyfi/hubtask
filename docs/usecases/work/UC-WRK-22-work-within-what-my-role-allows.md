---
id: UC-WRK-22
title: Work within what my role allows
context: work
actors: [PE-member, PE-guest, PE-child, PE-auditor, PE-agent]
deployments: [D2, D3, D4, D5, D6, D7]
serves: [P-01, P-05, P-11, P-12]
state: partial
tasks: [B-02, F2-07]
checked_by: [core/domain/service/Authorization_test.go, core/domain/service/ItemAccess_test.go, test/integration/tenant_boundary_test.go, test/integration/assignment_test.go]
---

# Work within what my role allows

## Goal

Whatever role a person holds where they are — member, contributor, viewer, guest — every screen of
their work offers exactly what that role lets them do there, so that they never press a button only
to be told no, and never see a trace of what they may not read.

## Story

The club's treasurer is a *viewer* on the events hub: she reads everything and changes nothing,
and the entries show her no checkbox that could be ticked, no title that could be edited, no
comment field. A *contributor* sees the whole collection but can change only what is on him; on
the rest the controls are absent and a short line says why. A *guest* shared on one task reads it
and comments, and sees nothing else of the workspace. An *auditor* sees no content at all. A link
to an entry somebody holds nothing on answers "not found", in the same words as an entry that does
not exist.

## How to check

1. A viewer is shown no control that changes an entry — no completion checkbox that can be
   ticked, no editable title or notes, no add, move, assign, label, date, reminder, recurrence,
   cover, field or comment control.
2. A contributor sees those controls on the entries they are responsible for, and on the others
   sees them absent with one line saying the entry is not theirs.
3. A guest on one entry sees that entry, its comments and a comment field, and no control that
   changes the entry; nothing of the rest of the workspace appears anywhere.
4. A member, contributor, viewer or guest is shown no control that changes the shape of the
   workspace — creating hubs or collections, columns, labels, fields, policies, templates or views
   wider than their own — and no administration.
5. An entry, a collection or a hub on whose path the person holds nothing answers as not found, in
   the words a missing one produces, through every channel.
6. An auditor opening the work area reads no hub, collection, entry or comment.
7. The screen decides what to offer from the role matrix the server publishes, not from a table in
   the client, so that a changed matrix changes the screen without a new release.
8. What the screen withholds, the server refuses as well: removing the screen's restraint by
   calling the API directly gains the person nothing.

## Where it ends

* Roles are the seven the matrix knows ([domain model §3.2](../../architecture/domain-model.md));
  no custom roles.
* Rights only add up — a narrower role in one place never removes a wider role from above.
* Giving somebody a role is [UC-WRK-21](./UC-WRK-21-share-a-hub-a-collection-or-one-entry.md).
* A simpler view designed for children is not asked for here; a child uses the same screens with
  a narrower role.

## Today

* Check 1: not met in the web app — the entry page gates its controls on archiving and the type's capabilities, never on the role, so a viewer sees every control and meets the server's refusal after pressing it, tracked in #1080.
* Check 2: not met in the web app — the entry page never asks the role, so a contributor sees the same controls on every entry, tracked in #1080.
* Check 3: not met in the web app — the entry page never asks the role, so a guest sees the controls that change the entry, tracked in #1080.
* Check 4: not met in part — *Create hub*, *Create collection*, the board's columns and the labels dialog are offered without asking the role (UC-WRK-01, UC-WRK-05, UC-WRK-09), tracked in #1080.
