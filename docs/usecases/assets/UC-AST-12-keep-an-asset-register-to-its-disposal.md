---
id: UC-AST-12
title: Keep an asset register to its disposal
context: assets
actors: [PE-admin, PE-member, PE-auditor]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-18]
state: built
tasks: []
checked_by: [core/domain/model/cmdb/Assurance_test.go, core/application/service/cmdb/Assurance_test.go, core/application/service/sync/InitialSyncCmdb_test.go, test/integration/cmdb_assurance_test.go, apps/webapp/src/lib/data/cmdb.test.ts, apps/webapp/src/lib/data/replica.test.ts]
---

# Keep an asset register to its disposal

## Goal

What an object cost, where it came from, who holds it and how it left is kept with the configuration item it is, from the request to its secure disposal, and the item's lifecycle follows.

## Story

A new laptop is requested for Kai. Dana records it as ordered, receives it into the stockroom with its tag, cost and warranty, and deploys it to Kai. Three years on it is retired, wiped and disposed of; the disposal names the certificate from the recycler.

## How to check

1. A CI's asset record holds its tag, cost in minor units with its currency, purchase day, supplier, warranty end, contract (a contract CI of the hub), stockroom (a location CI of the hub), the person holding it and the request entry it came from; a cost without a currency and a warranty ending before the purchase are refused.
2. An asset moves requested → ordered → in stock → deployed → retired → disposed, deployed back to stock and retired back to stock; it is deployed only to a person or a stockroom, and the CI's lifecycle stage follows each move.
3. Disposing needs its method - wiped, destroyed, recycled, returned, sold - and the reference of the certificate that proves it; a disposed asset moves no more, and nothing is deleted.
4. Asset records travel to a device filed under their hub.
5. The *Assets* screen lists a hub's register with its states and warranties, and a CI's screen registers its asset and moves it, the disposal with its certificate.

## Where it ends

* No purchase order, no depreciation, no licence entitlements (research A4) - the request is an entry of the service desk.
