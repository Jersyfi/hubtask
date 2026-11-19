---
id: UC-AST-10
title: Have owners attest our inventory
context: assets
actors: [PE-admin, PE-member, PE-auditor]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-18]
state: verified
tasks: []
checked_by: [core/domain/model/cmdb/Assurance_test.go, core/application/service/cmdb/Assurance_test.go, core/application/service/cmdb/Confirm_test.go, core/application/service/cmdb/Remind_test.go, core/application/service/notification/RecordAttestation_test.go, test/integration/cmdb_assurance_test.go, test/integration/notification_test.go, apps/webapp/src/lib/data/cmdb.test.ts, apps/webapp/e2e/inventorycare.test.mjs, apps/webapp/e2e/inventoryobject.test.mjs]
---

# Have owners attest our inventory

## Goal

The people who own objects confirm, at least once a year, that what the inventory records about them is true, and the workspace can show how far that review has come and who answered what.

## Story

Dana starts the annual review of the servers and virtual machines whose review falls due this quarter. Bert owns host1 and confirms it; vm1 was decommissioned last month, so Dana disputes it. The campaign shows two of two answered, and vm1's review is due today.

## How to check

1. A campaign of a hub asks every active object of the object types named - with a day given, only those whose review falls due by then or was never set - once, of its owning person; the set is fixed when it starts, and a campaign asking nothing is refused.
2. The person asked answers with READ on the hub, anybody else needs WRITE_ITEMS; CONFIRMED moves the object's next review by the campaign's period (twelve months unless it says otherwise), DISPUTED makes it fall due the same day.
3. A campaign's score counts the objects asked, confirmed, disputed, pending and - after the due day - overdue, and the share answered; every object's verdict names who gave it and when, and each answer is in the audit trail.
4. A closed campaign takes no answer and keeps its score.
5. The *Inventory* screen's *Upkeep* shows the open attestation round with its score - confirmed of asked, replies “not right” - or starts one and counts the earlier rounds; its campaigns part lists a hub's campaigns with their scores, starts one, and lets the person asked confirm or dispute each object.
6. An object is confirmed outside a campaign - by its owning person with READ on the hub, anybody else with WRITE_ITEMS: its next review moves twelve months on (or the months asked, 1 to 60) from the workspace's day, it records who confirmed it and when, as a CONFIRMED answer does, and a pending question of an open campaign about it is answered CONFIRMED; the object page's head offers *Confirm* as its primary act beside the review day and says when it was confirmed, never on a phone or to a reader who does not own it.
7. A member who may write reminds the owning people of the objects an open campaign still waits for: each is told once per reminder, by mail as their notification setting for rounds allows, how many of their objects wait; the answer says how many were reminded and the audit trail the count alone; with nobody to remind it is refused `cmdb.nobody_to_remind`, a closed campaign is refused. *Upkeep*'s round and the campaigns part offer *Remind* while objects wait.

## Where it ends

* No campaign that starts itself on a schedule, no reminder that sends itself - a member reminds the people asked; the review day on each object and the health's *reviews overdue* say what is due.
