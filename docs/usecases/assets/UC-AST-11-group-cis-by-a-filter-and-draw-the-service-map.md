---
id: UC-AST-11
title: Group configuration items by a filter and draw the service map
context: assets
actors: [PE-admin, PE-member, PE-handler, PE-auditor]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-18, P-19]
state: built
tasks: []
checked_by: [core/domain/model/cmdb/Assurance_test.go, core/application/service/cmdb/Assurance_test.go, core/application/service/sync/InitialSyncCmdb_test.go, test/integration/cmdb_assurance_test.go, apps/webapp/src/lib/data/cmdb.test.ts, apps/webapp/src/lib/data/replica.test.ts]
---

# Group configuration items by a filter and draw the service map

## Goal

A set of configuration items is defined by what they are - "every virtual machine whose app is crm" - rather than by listing them, may stand for what a service rests on, and the services of a hub are drawn with everything they rest on, layer by layer.

## Story

The CRM's machines come and go with the load. Dana defines the group "CRM machines" as every virtual machine in use whose attribute app is crm and lets it stand for the CRM application service. The service map shows the CRM offering, the application service and, beneath, every machine of the group and the hosts they run on.

## How to check

1. A group of a hub has a key, a name and a filter - classes with those below them, stages, a text the name holds, attribute values to equal without case -, at least one condition; it holds the CIs the filter matches now, merged ones never.
2. A group may stand for a service CI of its hub; its members are then drawn beneath that service in the service map.
3. The service map of a hub places every active service and offering and everything whose failure they suffer in layers - offerings, services, application services, software and data, virtual and cloud resources, what they stand on - with the relations carrying impact between them, up to 2,000 CIs.
4. A hub's groups travel to a device filed under the hub.
5. The *Assets* screen lists the groups with their members, defines one, and draws the service map in its layers.

## Where it ends

* No discovered or traffic-based service map (research I5), no group of hand-picked CIs.
