---
id: UC-AST-12
title: Keep the register record of a device or licence to its disposal
context: assets
actors: [PE-admin, PE-member, PE-auditor]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-18]
state: verified
tasks: []
checked_by: [core/domain/model/cmdb/Assurance_test.go, core/application/service/cmdb/Assurance_test.go, core/application/service/cmdb/AssetMoves_test.go, core/application/service/sync/InitialSyncCmdb_test.go, test/integration/cmdb_assurance_test.go, test/integration/cmdb_work_test.go, apps/webapp/src/lib/data/cmdb.test.ts, apps/webapp/src/lib/data/replica.test.ts, apps/webapp/e2e/assetsscreen.test.mjs, core/application/service/cmdb/Configuration_test.go, apps/webapp/e2e/inventoryobject.test.mjs, apps/webapp/src/lib/assets/objectpage.test.ts]
---

# Keep the register record of a device or licence to its disposal

## Goal

What an object cost, where it came from, who holds it and how it left is kept with the object it is, from the request to its secure disposal, and the object's lifecycle follows.

## Story

A new laptop is requested for Kai. Dana records it as ordered, receives it into the stockroom with its tag, cost and warranty, and deploys it to Kai. Three years on it is retired, wiped and disposed of; the disposal names the certificate from the recycler.

## How to check

1. An object's register record holds its tag, cost in minor units with its currency, purchase day, supplier, warranty end, contract (a contract object of the hub), stockroom (a location object of the hub), the person holding it and the request entry it came from; a cost without a currency and a warranty ending before the purchase are refused. The record keeps the moment it was registered with the state it started in, who registered it and with whom or in which stockroom; one registered before these moments were kept answers its state alone, with no date invented.
2. A register record moves requested → ordered → in stock → deployed → retired → disposed, deployed back to stock and retired back to stock; it is deployed only to a person or a stockroom, and the object's lifecycle stage follows each move. Each move keeps its moment, who moved it and to whom or which stockroom, and the record answers its life oldest first.
3. Disposing needs its method - wiped, destroyed, recycled, returned, sold - and the reference of the certificate that proves it; a disposed object moves no more, and nothing is deleted.
4. Register records travel to a device filed under their hub.
5. The *Inventory* screen lists a hub's devices and licences with their states and warranties, and an object's page registers its record and moves it, and shows the device's life from requested to disposed - each step reached with its day and its stockroom or person, the disposal with its certificate.

## Where it ends

* No purchase order, no depreciation, no licence entitlements (research A4) - the request is an entry of the service desk.
