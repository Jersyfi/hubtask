---
id: UC-AST-01
title: Keep an inventory of what our services run on
context: assets
actors: [PE-admin, PE-member, PE-auditor]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-18]
state: built
tasks: []
checked_by: [core/domain/model/cmdb/Cmdb_test.go, core/application/service/cmdb/Cmdb_test.go, test/integration/cmdb_repository_test.go, apps/webapp/src/lib/data/cmdb.test.ts, core/application/service/work/LinksCi_test.go, core/application/service/cmdb/Backlinks_test.go, core/application/service/cmdb/Changes_test.go, core/application/service/sync/InitialSyncCmdb_test.go, apps/webapp/src/lib/data/replica.test.ts, core/application/archive/Carried_test.go, apps/webapp/e2e/assetsscreen.test.mjs, apps/webapp/src/lib/data/inventoryquery.test.ts, apps/webapp/e2e/inventorystock.test.mjs, apps/webapp/e2e/inventoryobject.test.mjs, apps/webapp/src/lib/assets/objectpage.test.ts]
---

# Keep an inventory of what our services run on

## Goal

A hub holds the objects a team runs - hardware, virtual and cloud resources, software, business and technical services with their offerings, locations, contracts - each of an object type with typed attributes, related to the others, with an owner, a location, a protection need and a review date, so that one inventory answers ISO/IEC 27001 A.5.9, NIS2 and the BSI's structure analysis at once.

The inventory is an object database, not merely an inventory tool: it maps any structure as related objects - hardware, software, services, contracts, locations, people's equipment, anything a process needs - so that processes can be automated over connected information (the owner's decision of 2026-11-14).

## Story

Dana runs IT for a practice of forty people. She records the booking application as an application service, the two virtual machines it runs on and the server that hosts them, names the IT group as the owner of each, sets the booking service's availability need to very high and the next review to next March.

## How to check

1. A member who may write in a hub makes an object of an object type that is not abstract with typed attributes; an attribute the object type does not declare, or a value of the wrong kind, is refused with `cmdb.attribute_unknown` or `cmdb.attribute_value_invalid`, and an object type that holds no objects of its own with `cmdb.class_not_instantiable`.
2. Making an object the hub's identifiers already name makes nothing and is refused with `cmdb.ci_exists`, naming that object.
3. Two objects of one hub are related by a type of the catalogue whose object type pairs allow them; a pair the type does not join is refused with `cmdb.relation_not_allowed`.
4. An object carries a lifecycle stage and status, an owner (a person or a group), a location (an object of a location object type in the same hub), a protection need for confidentiality, integrity and availability, a review date and a quantity; a dimension fixed against inheritance without a reason is refused with `cmdb.protection_reason_required`.
5. Reading an object answers its relations both ways and the attribute's source that last wrote each value.
6. A member of another hub, or of another workspace, finds none of the hub's objects, relations or identifiers.
7. The workspace's archive holds every object type, relation type, source, object, identifier, relation, run, hold and duplicate, and a restore brings them back (tenant-export.md §6).
8. The *Inventory* screen of a hub lists its objects by object type, and an object's page shows its stage as a step bar, its attributes with the source that wrote each, its relations as a picture or as a list, and its owner, location, protection need and review in its rail - each editable there by a member who may write, nothing editable for a reader or on a phone.
9. An entry's notes or comments, a page or a whiteboard point at an object with `hubtask:ci/<id>`; the reference shows the object's name, object type and stage to a reader of its hub and only its address to anyone else, and the object's screen lists what points at it that the reader may see.
10. A device that holds a copy of the workspace reads a hub's inventory while the server is away: the objects by object type, an object with its relations, the protection need it inherits and its impact, and the object types and relation types; only a hub the reader may read reaches the device, every write of an object or a relation reaches it, and nothing is written offline.
11. Writing a comment or a page, a member mentions an object of a hub they may read, found by its name, inline or as a card; an object of a hub out of their reach, or one merged away, is not offered.
12. A member who may write puts files on an object - chosen or dropped on its *Files* section, uploaded as every file is - and takes them off; every reader of the hub sees them as a grid with previews or as a list and opens them in the one file viewer, paging with ← → over the object's files; a member of another hub reads none of them, and the files go with the object.
13. An object's *History* tells, newest first, who made it, changed it by hand or let a source change it, moved its stage, owner, location or protection need, related or unrelated it, confirmed or attested it, registered or moved its asset, and put a file on it or took one off, and when; whoever reads the hub reads it, it goes with the object, and on a phone the files and the history are read and nothing is written.

## Where it ends

* The register record beside the object (cost, purchase, warranty, disposal) is UC-AST-12 and software counted by its product UC-AST-13; no licence position against entitlements - the following release.
* No deletion of an object: an object is retired; deleting one is the owner's decision.
