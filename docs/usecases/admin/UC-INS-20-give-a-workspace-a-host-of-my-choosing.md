---
id: UC-INS-20
title: Give each workspace a host of my choosing
context: admin
actors: [PE-operator, PE-scripter]
deployments: [D4, D7]
serves: [P-01, P-05, P-08, P-09, P-10]
state: specified
tasks: [CD-02, CD-03, CD-04, CD-05]
checked_by: []
---

# Give each workspace a host of my choosing

## Goal

An operator who runs several workspaces without a wildcard certificate gives each one a host of
its own - `tasks-hr.corp.example`, `tasks-sales.corp.example` - and every workspace behaves there
exactly as it would on a subdomain.

## Story

A company's IT runs one installation for its legal entities. Its certificates are issued per host,
so it sets the installation to hosts it assigns. Creating a workspace asks for the host instead of
a short name under the installation's address; IT adds the DNS record and the certificate as it
does for every other service.

## How to check

1. An installation in multi mode set to assigned hosts asks for the workspace's host when a
   workspace is created - in the installation's workspaces, with `hubctl admin tenant create` and
   through the API, with the same checks - and no longer for a subdomain.
2. The workspace answers at its assigned host: its sign-in card shows that host, and invitations
   and every other mail link to it.
3. A host that another workspace uses, the installation's own host, or one that is not a valid
   host name is refused at the field with a code naming the reason; no DNS proof is asked.
4. Changing an assigned host is a change of address with every check of UC-INS-19.
5. A sign-in at a provider returns through the installation's one return address and lands on the
   workspace's assigned host.
6. In single mode, and in multi mode with subdomains, nothing about assigned hosts is shown.

## Where it ends

* The DNS record and the certificate for an assigned host are the operator's, as for any host of
  theirs; Hubtask checks no DNS for it.
* An installation uses subdomains or assigned hosts, not both; switching an installation that
  already has workspaces between the two is not offered.
* A workspace's own domain, set up by its owner, is UC-INS-15.

## Today

* Check 1: not met — there is no setting for assigned hosts; creating a workspace asks for a short name only.
* Check 2: not met — nothing resolves a workspace through a host other than its subdomain.
* Check 3: not met — there are no assigned hosts to refuse.
* Check 4: not met — the address cannot change (UC-INS-19).
* Check 5: not met — the provider's return forwards only to a host beneath the installation's own.
