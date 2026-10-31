---
id: UC-AST-06
title: See how healthy our inventory is
context: assets
actors: [PE-admin, PE-auditor]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-18]
state: built
tasks: []
checked_by: [core/domain/model/cmdb/Cmdb_test.go, core/application/service/cmdb/Cmdb_test.go, apps/webapp/src/lib/data/cmdb.test.ts]
---

# See how healthy our inventory is

## Goal

A hub's inventory is judged - complete, correct, owned, reviewed - per class and in all, and the configuration items found wanting are named, so that data quality is known before anybody relies on it.

## Story

Before the yearly review Dana opens the inventory's health: completeness is 82 %, eleven CIs have not been confirmed by any source for two months, three application services run on nothing recorded, and two CIs have no owner. She works through the list.

## How to check

1. Completeness counts the required and recommended attributes of each active CI's class and its owner; correctness counts the CIs that are stale, orphaned or in an open duplicate; retired and disposed CIs do not count.
2. A CI is stale when no source has confirmed it for its class's staleness period (60 days unless a class says otherwise).
3. A CI is an orphan when it lacks a relation its class requires - an application service with nothing it runs on or depends on, a virtual machine hosted on nothing, an offering of no service.
4. The answer counts CIs without an owner and reviews overdue, per class and in all, and names the CIs found wanting with the kinds of problem and the required attributes missing.
5. An administrator defines a class of the workspace's own beneath a shipped one with its attributes (optional, recommended, required; identifying or not), identifier rules, required relations and staleness period; a shipped class is not changed.
6. The *Assets* screen shows the health with its figures per class and opens a CI found wanting.
7. Each active service - business, technical, application service, offering - is scored by the share of the active CIs it rests on, itself and everything whose failure it suffers, that are found wanting in nothing; the worst comes first, and the *Assets* screen opens each.
8. A relation a program reported is unconfirmed when that program has not reported it again within the staleness period of its source CI's class; a relation a person made never is. Health counts them; nothing is removed.
9. Each source that ran in the hub carries a conflict rate over the hub's latest twenty runs: of the attributes it reported with a value other than the stored one, the share a better source kept.

## Where it ends

* Owners confirming their CIs in a campaign is UC-AST-10; no lifecycle policies - the following release.
* An unconfirmed relation is counted, not expired: removing what a program no longer reports is a lifecycle policy.
