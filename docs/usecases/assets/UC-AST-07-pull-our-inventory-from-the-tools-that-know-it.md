---
id: UC-AST-07
title: Pull our inventory from the tools that already know it
context: assets
actors: [PE-admin, PE-integrator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-18]
state: built
tasks: []
checked_by: [core/domain/model/cmdb/Connector_test.go, core/application/service/cmdb/Connectors_test.go, infrastructure/connector/Pullers_test.go, presentation/worker/CmdbConnectorPull_test.go, test/integration/cmdb_repository_test.go, apps/webapp/src/lib/data/cmdb.test.ts]
---

# Pull our inventory from the tools that already know it

## Goal

The servers, devices and machines GLPI, NetBox, Intune, Entra ID, Hetzner Cloud or an OpenStack cloud already know arrive in a hub's inventory by themselves, on a schedule, through the same reconciliation every source writes through - with a credential nobody can read back.

## Story

Dana connects the hub *Data centre* to NetBox with a token, ranks NetBox above the scanner, tries it as a dry run and switches on an hourly pull. The next morning the switches and servers are there with their serials and addresses; when NetBox is down for an hour, the connector says so by a code and the next run fetches what it missed. She adds the company's Intune with its app registration and the Hetzner project with its token.

## How to check

1. An administrator defines a connector in a hub - its kind (GLPI, NetBox, Intune, Entra ID, Hetzner Cloud, OpenStack), the connector source it writes as, its https address, the kind's settings, further hosts it may reach and a period of 15 minutes to a week or none - and a source that is not of kind `CONNECTOR` is refused with `cmdb.connector_source_not_connector`.
2. The credential is set on its own, sealed at rest under a purpose bound to the connector, and never answered by any door: a connector shows only whether one is set.
3. A run pulls page by page from where the last run ended - from the beginning when a full walk is asked - and every page goes through the reconciliation of UC-AST-02 as the connector's source; a dry run answers the first page's decisions and writes nothing.
4. A call reaches only the connector's base address's host, its identity provider's and the hosts it allows; any other is refused with `connector.host_not_allowed`, and every call leaves through the guarded client with a deadline.
5. A source that refuses the credential fails the run with `connector.unauthorised`; one that answers 429 fails it with `connector.rate_limited` and the next run waits as long as it asked; a failure keeps the cursor, so nothing is skipped, and the run's outcome, error code and figures stay on the connector.
6. A scheduled connector is pulled when due as the person who last defined it, again every period, and switches itself off after ten failed runs in a row.
7. The *Assets* screen lists a hub's connectors, defines one, sets its credential, runs it and shows how its last run ended.
8. A run that fails or fetches nothing fires the hub's alert about that connector's runs - one alert, counted again by every further bad run - and the next run that fetches resolves it; a dry run changes nothing. The *Alerts* section shows it with the failure's words.

## Where it ends

* No relay agent for sources inside a customer's network, no versioned mapping packs, no Snipe-IT, i-doit, Jamf, Wazuh or cloud inventories of AWS, Azure or GCP, and no NetBox change webhook - the following release (research itsm-cmdb-ai §4.6, C7, C9).
* A connector is switched off, never deleted: deleting one would take its alerts with it, which is the owner's decision (irrecoverable deletion).
