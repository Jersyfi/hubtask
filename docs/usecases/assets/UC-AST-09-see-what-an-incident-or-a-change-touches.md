---
id: UC-AST-09
title: See what an incident or a change touches
context: assets
actors: [PE-handler, PE-member, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-18, P-19]
state: built
tasks: []
checked_by: [core/domain/model/cmdb/Assurance_test.go, core/application/service/cmdb/Assurance_test.go, core/application/service/sync/InitialSyncCmdb_test.go, test/integration/cmdb_assurance_test.go, apps/webapp/src/lib/data/cmdb.test.ts]
---

# See what an incident or a change touches

## Goal

An incident, a change or any other entry names the configuration items it is about, and whoever works on it sees which services and offerings their failure reaches, which other open entries are about the same things, and which changes are planned on them at the same time.

## Story

Kai opens the incident "Booking is slow" and names the booking application as affected. The entry shows the booking offering the reception depends on, and the change "Patch host1" planned for tomorrow on the host the application runs on - Kai links the two before the patch goes ahead.

## How to check

1. A person who may change an entry names a CI they may read as affected, up to fifty per entry; naming one again changes nothing, and a CI of a hub out of their reach answers not found.
2. The entry's affected CIs are listed with the active services and offerings their failure reaches, up to eight steps, only those the reader may read.
3. The other open entries naming an affected CI, or a CI it reaches, are listed - only those the reader may see.
4. An entry planned with a start and a due collides with another entry planned on the same CI, or on one its CIs reach, in an overlapping window; the collision says whether both name the CI itself.
5. An entry's affected CIs travel to a device filed under the CI's hub and never to a reader of another hub.
6. The entry's screen shows its affected CIs, the services they reach, the related entries and the collisions, and a writer names and removes a CI there.

## Where it ends

* No CI suggested for an incident by an agent (M9's CI suggestion), no rule that routes by the affected offering.
