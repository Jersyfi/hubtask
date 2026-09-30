---
id: UC-INS-07
title: Create a workspace for a customer
context: admin
actors: [PE-operator, PE-platform, PE-scripter]
deployments: [D5, D6, D7]
serves: [P-05, P-08, P-15]
state: partial
tasks: [H-06, SI-17]
checked_by: [core/application/service/admin/Provision_test.go]
---

# Create a workspace for a customer

## Goal

An operator — by hand, by script or through a platform — creates a workspace with a name, an
address and an owner, and the owner receives a way in.

## Story

*Installation → Workspaces → New workspace*: name, short name (the address), the owner's address.
The owner receives an invitation; the operator can also copy the owner's link once, for platforms
that deliver it themselves.

## How to check

1. The screen, `hubctl admin tenant create` and `POST /admin/tenants` create the same workspace with
   the same checks.
2. A short name that is taken, reserved or not a valid address label is refused at the field.
3. The owner's way in is delivered by mail and, once, as a link to copy.
4. On a single-tenant installation, *New workspace* is not offered anywhere.
5. The creation is in the instance journal and in the new workspace's own trail.

## Where it ends

* No self-registration by the public; a platform in front of Hubtask does that
  ([NG-billing](../../vision/non-goals.md)).
* The plan a workspace starts on is UC-INS-13.

## Today

* **Check 4 fails:** *New workspace* is offered in single mode and fails with
  `admin.multi_mode_required`.
