---
id: UC-AST-08
title: Turn monitoring alerts into entries on the affected configuration item
context: assets
actors: [PE-admin, PE-integrator, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-18, P-19]
state: partial
tasks: []
checked_by: [core/domain/model/cmdb/Connector_test.go, core/application/service/cmdb/Connectors_test.go, presentation/intake/AlertIntake_test.go, test/integration/cmdb_repository_test.go]
---

# Turn monitoring alerts into entries on the affected configuration item

## Goal

When Alertmanager, Zabbix or Checkmk raises a problem, the hub knows it once - bound to the configuration item it concerns - and an entry for it is opened while it fires and completed when it is resolved.

## Story

Ole points the team's Alertmanager at the hub's intake with the secret Hubtask drew for it. A disk fills up on *web01*: an entry *disk full* opens in the collection *Incidents*, pointing at the CI *web01*. The alert fires every five minutes and stays one entry. When the disk is cleaned, the entry is completed by itself.

## How to check

1. An administrator defines an alert intake in a hub (Alertmanager, Zabbix or Checkmk), optionally with a collection of that hub; setting its credential draws a secret and answers it once.
2. A delivery is taken in only with the intake's secret as a bearer token, or signed with it - an HMAC-SHA-256 over the moment and the body - inside five minutes; a wrong secret, a stale signature, a replayed delivery and an unknown or switched-off intake all answer the same `404` with `cmdb.intake_not_found`.
3. Each alert is kept once per intake by its key: firing again counts it, a resolution resolves it, firing after that reopens it; a resolution of a problem never seen records nothing.
4. An alert is bound to the one CI of the hub its host names - by host name (with a port or a domain stripped), address or CI name - and to none when no lookup names exactly one.
5. With a collection, a firing alert opens one entry there through the same use case a person creates with, its notes pointing at the CI; its resolution completes the entry and a reopening reopens it. An entry that cannot be written leaves the alert kept and is counted in the receipt.
6. A member who may read the hub lists its alerts, the latest first, narrowed to firing or resolved.
7. The *Assets* screen shows a hub's alerts with their CI and entry, and an intake's address and secret once.

## Where it ends

* No on-call schedules, routing or escalation, no deduplication across intakes, no affected CIs on an entry beyond the reference in its notes (S-18's incidents) - the following release.

## Today

* Check 7: the *Assets* screen has no *Alerts* section yet (S-17, the web app's part).
