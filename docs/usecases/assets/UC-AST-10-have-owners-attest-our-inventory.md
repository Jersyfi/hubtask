---
id: UC-AST-10
title: Have owners attest our inventory
context: assets
actors: [PE-admin, PE-member, PE-auditor]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-18]
state: partial
tasks: []
checked_by: [core/domain/model/cmdb/Assurance_test.go, core/application/service/cmdb/Assurance_test.go, test/integration/cmdb_assurance_test.go]
---

# Have owners attest our inventory

## Goal

The people who own configuration items confirm, at least once a year, that what the inventory records about them is true, and the workspace can show how far that review has come and who answered what.

## Story

Dana starts the annual review of the servers and virtual machines whose review falls due this quarter. Bert owns host1 and confirms it; vm1 was decommissioned last month, so Dana disputes it. The campaign shows two of two answered, and vm1's review is due today.

## How to check

1. A campaign of a hub asks every active CI of the classes named - with a day given, only those whose review falls due by then or was never set - once, of its owning person; the set is fixed when it starts, and a campaign asking nothing is refused.
2. The person asked answers with READ on the hub, anybody else needs WRITE_ITEMS; CONFIRMED moves the CI's next review by the campaign's period (twelve months unless it says otherwise), DISPUTED makes it fall due the same day.
3. A campaign's score counts the CIs asked, confirmed, disputed, pending and - after the due day - overdue, and the share answered; every CI's verdict names who gave it and when, and each answer is in the audit trail.
4. A closed campaign takes no answer and keeps its score.
5. The *Assets* screen lists a hub's campaigns with their scores, starts one, and lets the person asked confirm or dispute each CI.

## Where it ends

* No campaign that starts itself on a schedule, no reminder mail to the people asked - the review day on each CI and the health's *reviews overdue* say what is due.

## Today

* Check 5: not met — the web app has no campaign screen yet.
