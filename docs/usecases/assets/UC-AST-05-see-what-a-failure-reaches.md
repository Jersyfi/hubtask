---
id: UC-AST-05
title: See what a failure reaches
context: assets
actors: [PE-admin, PE-member, PE-handler, PE-auditor]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-18, P-19]
state: built
tasks: []
checked_by: [core/domain/model/cmdb/Cmdb_test.go, core/application/service/cmdb/Cmdb_test.go, test/integration/cmdb_repository_test.go, apps/webapp/src/lib/data/cmdb.test.ts]
---

# See what a failure reaches

## Goal

From any configuration item a person sees what suffers when it fails - and which services and offerings among them - or what it depends on, and each item's protection need is inherited from what depends on it.

## Story

The host of the two virtual machines needs a firmware update. Before planning it, Dana opens the host's impact: both machines, the booking application running on them, and the booking offering the practice's reception depends on. Because the booking service needs very high availability, the host inherits that need.

## How to check

1. A relation type says which end suffers when the other fails (`NONE`, `SOURCE`, `TARGET`, `BOTH`); the shipped catalogue is depends on, runs on, hosted on, installed on, contains, connects to, member of and offering of.
2. Downstream impact names every CI that suffers when one fails, once each at its nearest depth, up to eight steps, with the active services and offerings among them; upstream names every CI whose failure it suffers.
3. A CI's effective protection need is, per confidentiality, integrity and availability, the highest of its own and that of everything that suffers when it fails; a dimension its owner fixed with a reason keeps its own.
4. The walk stays in the CI's hub and workspace.
5. The *Assets* screen shows a CI's impact as a list grouped by depth and its effective protection need beside its own.

## Where it ends

* No drawn service map, no impact on an incident or change (they come with the service desk, S-18), no change collision - the following release.
