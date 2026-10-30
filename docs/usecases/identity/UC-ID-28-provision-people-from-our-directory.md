---
id: UC-ID-28
title: Provision people and groups from our directory
context: identity
actors: [PE-admin, PE-integrator]
deployments: [D3, D4, D6, D7]
serves: [P-01, P-02, P-03, P-12, P-16]
state: verified
tasks: []
checked_by: [core/domain/model/identity/Directory_test.go, core/application/service/identity/Directory_test.go, presentation/rest/Scim_test.go, test/integration/directory_test.go]
---

# Provision people and groups from our directory

## Goal

A company's directory - Entra ID, Okta, Keycloak, Google - keeps the workspace's people and groups
over SCIM 2.0: a person who joins is there before their first day, a person who leaves can no
longer act the moment the directory says so, and the groups follow the directory's - without an
administrator repeating any of it by hand, and without the directory being able to do more than an
administrator may.

## Story

The IT administrator creates a service account *Directory* (UC-ID-17), gives it a token with
`members:write`, and enters the workspace's SCIM address `…/api/v1/scim/v2` and the token in Entra
ID. Entra provisions Lena: she is in the workspace's member list as invited, holds the groups Entra
put her in and through them their roles, and becomes active at her first sign-in through the
workspace's provider. When Lena leaves the company, Entra switches her off: her sessions end, her
tasks and comments stay hers. The owner of the workspace cannot be switched off this way.

## How to check

1. `POST /scim/v2/Users` with an address creates an INVITED account under that address, holding
   the directory's `externalId`, answered `201` as a SCIM user with `active: true`; the same
   address again is refused `409` with `scimType: uniqueness`.
2. `active: false` (by PATCH, PUT or `DELETE /scim/v2/Users/{id}`) switches the person to
   DISABLED and ends their sessions; nothing of theirs is erased; provisioning the same address
   again switches the same account on rather than creating a second.
3. Switching off an account that holds `OWNER` on the workspace is refused with
   `directory.owner_kept`, and an account under a restriction of processing with
   `directory.restricted_kept`.
4. A `userName` other than the address the person signs in with is refused `400` with
   `scimType: mutability`; the display name is changed.
5. `GET /scim/v2/Users?filter=` finds a person by `userName`, `externalId` or `id` with `eq`; any
   other filter is refused `400` with `scimType: invalidFilter`, never answered as everybody.
6. A group's members are added and removed by PATCH - a value list or `members[value eq "…"]` -
   through the same rules as a group changed by hand; a PATCH that changes nothing writes nothing.
7. The directory's identifiers are unique in a workspace and neither read nor written from
   another workspace.

## Where it ends

* No password over SCIM: a provisioned person signs in through the workspace's provider.
* No bulk operations, sorting or ETags; filters are one equality.
* A group holding `OWNER` is not changed by a directory: its member change needs a person's
  step-up.
* SAML sign-in is UC-ID-30; LDAP synchronisation is not part of it.
