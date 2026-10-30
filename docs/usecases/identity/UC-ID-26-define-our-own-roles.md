---
id: UC-ID-26
title: Define our own roles from the permissions Hubtask knows
context: identity
actors: [PE-owner, PE-admin, PE-auditor, PE-member, PE-requester, PE-agent]
deployments: [D3, D4, D5, D6, D7]
serves: [P-05, P-06, P-07, P-08, P-11]
state: built
tasks: []
checked_by: [core/domain/service/PermissionCatalogue_test.go, core/application/service/identity/Roles_test.go, test/security/custom_roles_test.go, test/integration/custom_role_test.go, core/application/service/work/TakeStep_test.go, core/application/service/workflow/Workflows_test.go, apps/webapp/src/lib/data/roles.test.ts]
---

# Define our own roles from the permissions Hubtask knows

## Goal

A workspace whose way of working does not fit the seven shipped roles defines its own: a key, a
name, and permissions from the one closed catalogue Hubtask knows - permissions over a place, a
restricted field's permission, a workflow step's. It gives such a role to a person or a group at
the workspace, a hub or a collection, and it applies downwards as a shipped role does. Nobody hands
out a permission they do not hold, and the administration shows, for any person and place, what
they may do and through which role.

## Story

The service desk of a company wants *Service lead*: reads everything in the *Support* hub, shapes
its collections, may approve changes, but manages nobody. The administrator defines the role from
the catalogue, gives it to Lukas at the hub, and later answers the auditor's question "why may
Lukas edit the change workflow's approval?" from *Who may do what*: "role *Service lead* at hub
*Support*". An administrator who tries to add *delete a container* to the role is told that they
lack it themselves.

## How to check

1. An administrator defines a role with a lower-case key, a name and permissions from the
   catalogue (`GET /roles` lists the catalogue, the seven shipped roles with what each carries,
   and the workspace's own); an unknown permission or a key that spells a shipped role is refused
   with `roles.permission_unknown` or `roles.key_invalid`.
2. A role of the workspace's own is given to a person or a group at the workspace, a hub or a
   collection; the holder may do what it carries there and below, and nothing else - a role with
   `READ` and `WRITE_ASSIGNED` changes only the entries assigned to its holder, a role without
   `READ` sees no entry.
3. Nobody hands out a permission they do not hold: defining, changing, granting, revoking or
   deleting a role whose permissions the caller lacks at that place is refused with
   `access.permission_above_own`, naming the permissions lacking, and recorded in the trail.
4. A workflow step limited to roles may name a role of the workspace's own; the step permission
   `TAKE_STEP:<workflow>/<step>` opens the step to its holder whatever the step names.
5. `GET /access/explain` answers, for a person and a place, the roles that count there, where each
   is held and every permission they carry; given a permission, whether the person holds it and
   through which role. A person may ask about themselves; about somebody else it takes
   `MANAGE_MEMBERS` or `READ_CONFIGURATION` at the workspace and, for a hub or a collection,
   `READ` at that place - a place the caller does not reach answers not found.
6. *People & access → Roles* lists the shipped and the workspace's own roles with what each
   carries; an administrator defines, changes and deletes the workspace's own there, and is
   offered only the permissions they hold.
7. *People & access → Who may do what* shows a matrix of the catalogue's permissions against the
   roles, and, for a chosen person and place, which of them they hold and through which role.
8. *View as* on a person shows the administration's answer for them: their roles at a place, what
   they may do, and for a chosen permission, whether and why.
9. A role, its grants and its permissions are in the workspace's archive and restored with it.

## Where it ends

* The catalogue is closed: a workspace combines Hubtask's permissions, it does not invent new
  ones, and a permission of the catalogue keeps its meaning in every role that carries it.
* The seven shipped roles do not change; a custom role is no rung on their ladder, so it never
  hands out a shipped role.
* A custom role is held at a place, never on a single entry; sharing an entry stays a shipped
  role's.
* Hiding a field from readers is UC-ID-27.
