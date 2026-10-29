---
id: UC-ID-27
title: Hide a field from those who must not see it
context: identity
actors: [PE-owner, PE-admin, PE-member, PE-agent]
deployments: [D3, D4, D5, D6, D7]
serves: [P-05, P-08, P-11]
state: built
tasks: []
checked_by: [core/application/service/work/FormGate_test.go, core/application/service/work/FieldSight_test.go, core/domain/service/PermissionCatalogue_test.go, core/domain/model/work/CustomField_test.go, test/integration/custom_field_test.go, apps/webapp/src/lib/data/roles.test.ts]
---

# Hide a field from those who must not see it

## Goal

A field that only some people may touch - a cost, a salary band, a legal assessment - is marked
restricted. Only whoever holds its permission writes it, through every door; and, where the
workspace says so, only they read it.

## Story

The finance team adds the field *Cost* to the change requests. It is restricted: the change
manager and the members of *Finance*, whose role names `FIELD_WRITE:cost`, set it; a member who
edits the request's title leaves it as it is and is never asked for it; an agent with a member's
token is refused when it tries to write it.

## How to check

1. A field defined or changed with `restricted: true` is written only by holders of
   `FIELD_WRITE:<key>` along the entry's path - the owner, the administrators, and a role of the
   workspace's own that names it; anybody else's write of it is refused with `fields.restricted`
   on every door, whether or not the entry's type has a form, and a write that leaves the field as
   it was is no write of it.
2. Changing whether a field is restricted takes its permission at the workspace.
3. A field restricted for reading is absent from every answer to a person who lacks its read
   permission: the entry, a list, a query, the change stream, an export and a search.
4. The entry panel shows a restricted field read-only to whoever may not write it.

## Where it ends

* A calculated field is written by the server and is never restricted.
* A restriction narrows the shipped roles below the administrators; it widens nobody.
