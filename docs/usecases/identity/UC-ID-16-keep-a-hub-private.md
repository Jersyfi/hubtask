---
id: UC-ID-16
title: Keep a hub private, even from another administrator
context: identity
actors: [PE-member, PE-owner, PE-admin]
deployments: [D2, D3]
serves: [P-01, P-05, P-10]
state: specified
tasks: []
checked_by: []
---

# Keep a hub private, even from another administrator

## Goal

In a family or a small team, one person keeps a hub — a diary, a present list, a private project —
that the other adults cannot open, even though they help run the workspace.

## Story

Maria creates the hub *Presents* and marks it *Private — only me and people I add*. Her partner is
an administrator of the family workspace too; he sees neither the hub's contents nor its name. If
Maria leaves the workspace, the hub goes to the owner's trash with a notice, so nothing is lost.

## How to check

1. A private hub's contents and name are invisible to every workspace member not added to it,
   including owners and administrators of the workspace.
2. Administrators can see that private hubs exist and how much space they use, as a count — not
   their names.
3. Search, the overview, automation rules of others, exports by others and the calendar feed do
   not reach into a private hub.
4. When the hub's last member leaves, the hub is not deleted silently: it goes to the trash with
   the usual grace period, and the owner is told.
5. A backup and the workspace export still contain it — the data belongs to the workspace — and
   the export says so.

## Where it ends

* Not encryption: an operator with database access and the workspace's backups can still read it
  ([NG-e2e-encryption](../../vision/non-goals.md)).
* Not for audits: an auditor's configuration view counts private hubs but does not open them.

## Today

* **Not built.** Rights only add up in the authorization model; a workspace-level owner or
  administrator reaches every hub. It needs a decision (ADR) about a narrowing membership before a
  task can be cut.
