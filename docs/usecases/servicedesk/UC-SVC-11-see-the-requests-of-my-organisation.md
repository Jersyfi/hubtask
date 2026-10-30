---
id: UC-SVC-11
title: See the requests of my organisation in the portal
context: servicedesk
actors: [PE-requester, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-11, P-18]
state: built
tasks: []
checked_by: [core/domain/model/servicedesk/Organisation_test.go, core/application/service/config/ServiceDesk_test.go, core/application/service/servicedesk/Organisations_test.go, test/integration/service_desk_organisation_test.go, apps/webapp/src/lib/servicedesk/organisations.test.ts]
---

# See the requests of my organisation in the portal

## Goal

The people of one customer or one department see what their colleagues already asked the service
desk, so that the printer on floor 2 is reported once - where the administrator lets them. Who
belongs to an organisation follows from the mail domain of their address; nobody is assigned by
hand, and nobody sees more than their own until the organisation shares its requests.

## Story

Acme's people write from `acme.example`. Anna, the administrator, adds the organisation *Acme* with
that domain in *Administration → Service desk* and switches on *Members see each other's requests*.
Mehmet opens the portal of *IT help*, chooses *My organisation's requests*, and finds that Jonas has
already reported the blank pages; he reads Jonas's request and its public replies, but cannot answer
or rate it. When Jonas's address moves to another company, Mehmet no longer sees his requests.

## How to check

1. Somebody holding STRUCTURE at the workspace defines an organisation under a key with a name and
   one to twenty mail domains, normalised to small letters; a malformed or repeated domain is
   refused (`organisations.domain_malformed`, `organisations.domain_repeated`), a domain another
   organisation of the workspace holds is refused with `organisations.domain_taken`, and sharing is
   off unless it is switched on. Any member of the workspace lists the organisations.
2. A person belongs to the organisation whose domains hold the domain of their address - exactly
   that domain, never a subdomain -, read when they ask, so an address that moves takes them out.
3. In the portal, *My organisation's requests* (`scope=ORGANISATION`) lists the requests every
   active person of the requester's organisation raised in the collection, newest first, where the
   organisation shares its requests; otherwise it is refused with `requests.organisation_not_shared`.
   Every list names the requester's organisation, or none.
4. A requester reads a colleague's request of a sharing organisation with its public replies and
   goals, never the colleague's score; answering or rating it is refused as not found, and without
   sharing the colleague's request is not found.
5. *Administration → Service desk → Organisations* lists, adds, changes and deletes organisations;
   the portal of a collection offers *My requests* and *My organisation's requests* only to a
   requester whose organisation shares its requests.
6. The organisations are the `requester_organisations` section of the workspace's configuration
   document by their keys: exported with name, domains and sharing, applied through the same use
   cases, a change flagged as changing permissions, and a domain moved between two organisations
   in one apply given up before it is claimed. A published template leaves them out.

## Where it ends

* Requesters without an account (mail, public forms) belong to no organisation.
* An organisation is not a group for roles or approvals; it decides only what the portal shows.
