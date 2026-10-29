---
id: UC-AST-01
title: Keep an inventory of what our services run on
context: assets
actors: [PE-admin, PE-member, PE-auditor]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-18]
state: built
tasks: []
checked_by: [core/domain/model/cmdb/Cmdb_test.go, core/application/service/cmdb/Cmdb_test.go, test/integration/cmdb_repository_test.go, apps/webapp/src/lib/data/cmdb.test.ts, core/application/service/work/LinksCi_test.go, core/application/service/cmdb/Backlinks_test.go]
---

# Keep an inventory of what our services run on

## Goal

A hub holds the configuration items a team runs - hardware, virtual and cloud resources, software, business and technical services with their offerings, locations, contracts - each of a class with typed attributes, related to the others, with an owner, a location, a protection need and a review date, so that one inventory answers ISO/IEC 27001 A.5.9, NIS2 and the BSI's structure analysis at once.

## Story

Dana runs IT for a practice of forty people. She records the booking application as an application service, the two virtual machines it runs on and the server that hosts them, names the IT group as the owner of each, sets the booking service's availability need to very high and the next review to next March.

## How to check

1. A member who may write in a hub makes a configuration item of a class that is not abstract with typed attributes; an attribute the class does not declare, or a value of the wrong kind, is refused with `cmdb.attribute_unknown` or `cmdb.attribute_value_invalid`, and a class that holds no CIs of its own with `cmdb.class_not_instantiable`.
2. Making a CI the hub's identifiers already name makes nothing and is refused with `cmdb.ci_exists`, naming that CI.
3. Two CIs of one hub are related by a type of the catalogue whose class pairs allow them; a pair the type does not join is refused with `cmdb.relation_not_allowed`.
4. A CI carries a lifecycle stage and status, an owner (a person or a group), a location (a CI of a location class in the same hub), a protection need for confidentiality, integrity and availability, a review date and a quantity; a dimension fixed against inheritance without a reason is refused with `cmdb.protection_reason_required`.
5. Reading a CI answers its relations both ways and the attribute's source that last wrote each value.
6. A member of another hub, or of another workspace, finds none of the hub's CIs, relations or identifiers.
7. The workspace's archive holds every class, relation type, source, CI, identifier, relation, run, hold and duplicate, and a restore brings them back (tenant-export.md §6).
8. The *Assets* screen of a hub lists its CIs by class and shows one with its attributes, relations and compliance fields, editable by a member who may write.
9. An entry's notes or comments, a page or a whiteboard point at a CI with `hubtask:ci/<id>`; the reference shows the CI's name, class and stage to a reader of its hub and only its address to anyone else, and the CI's screen lists what points at it that the reader may see.

## Where it ends

* No financial asset record (cost, purchase, warranty) beside the CI, no software licence position - the following release.
* No deletion of a CI: a CI is retired; deleting one is the owner's decision.
