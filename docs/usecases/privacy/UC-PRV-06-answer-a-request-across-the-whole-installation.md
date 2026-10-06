---
id: UC-PRV-06
title: Answer a request across every workspace of the installation
context: privacy
actors: [PE-operator, PE-platform]
deployments: [D4, D5, D6, D7]
serves: [P-01, P-08, P-11]
state: partial
tasks: [E-10]
checked_by: [core/application/service/privacy/Export_test.go]
---

# Answer a request across every workspace of the installation

## Goal

When the operator of an installation is itself the one who must answer — a company running several
workspaces, a provider asked about its own customer accounts — it can collect a person's data from
every workspace that person is in, in one case, without reading any workspace's content itself and
without any workspace learning about the others.

## Story

A provider receives a request for access from somebody who has accounts in three of its customers'
workspaces. An operator records one installation-wide case by the person's address and names a
target. The job visits each workspace in turn, under that workspace's own rules, collects what
belongs to the person and writes it into one archive. Each of the three workspaces finds an entry
in its own trail saying that its data was collected for a request, and by whom. The web app does not
offer this; it says to raise such a request with whoever runs the installation.

## How to check

1. An installation-wide case needs the `admin:tenants` scope; without it, it is refused with
   `privacy.installation_scope_denied`.
2. An installation-wide case needs an address; an account identifier names somebody in one
   workspace only.
3. Starting the case writes one archive holding, from every workspace the address has an account
   in, the same kinds of data a single workspace's copy holds.
4. Every workspace visited gets a `dsr.collected` entry in its own trail.
5. The only thing asked across workspaces is which workspaces the address is a member of; the
   collection itself runs inside each workspace in turn.
6. The web app does not offer an installation-wide case and says whom to ask.

## Where it ends

* No installation-wide erasure: an erasure is carried out by each workspace's owner, who owns the
  work it destroys.
* No reading of a workspace's trail or content by the operator beyond what the archive is for
  ([NG-operator-reads-content](../../vision/non-goals.md)).
* A provider that is only a processor for its customers does not use this: each customer answers
  their own requests.

See [data-protection.md](../../architecture/data-protection.md) §4.

## Today

* Check 3: not met — the job carries the case out as a system actor without token scopes, and the collection asks it for `admin:tenants` again, so a started installation-wide case is refused by its own job; other workspaces are also matched by address only, which misses the person's account, entries and comments there, tracked in #1076.
* Check 4: not met — the `dsr.collected` entries are written by the collection loop, which is not reached while check 3 fails, tracked in #1076.
