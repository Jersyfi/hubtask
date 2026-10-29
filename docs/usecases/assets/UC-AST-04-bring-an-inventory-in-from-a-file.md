---
id: UC-AST-04
title: Bring an inventory in from a file
context: assets
actors: [PE-admin, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-18]
state: built
tasks: []
checked_by: [core/application/service/cmdb/Cmdb_test.go, apps/webapp/src/lib/data/cmdb.test.ts]
---

# Bring an inventory in from a file

## Goal

An inventory kept in a spreadsheet, or a software bill of materials, becomes configuration items through the same reconciliation - tried first without writing anything.

## Story

Dana exports the old asset spreadsheet as CSV, maps *Host* to the name, *Serial* to the serial number and the source's own key, and *Vendor* to the manufacturer, tries it as a dry run, sees one row without a host name reported, and imports it. The developers hand her the bill of materials of the booking application; its components become software CIs with their dependencies.

## How to check

1. A CSV table or a JSON array is read by a mapping - the class or a class column, the name column, the source-key column, columns to attributes, defaults - given with the import or saved on a file source; rows without a name are reported by number.
2. A CycloneDX or SPDX bill of materials becomes CIs of the class `software` identified by package URL, CPE or name and version, and its dependencies become `depends_on` relations.
3. An import runs through the reconciliation of UC-AST-02, with its dry run, holds and explanation.
4. A file above 5 MiB is refused with `cmdb.file_too_large`, and one that cannot be read in its format with `cmdb.file_unreadable`.
5. The *Assets* screen takes a file, shows the dry run's decisions row by row and imports it.

## Where it ends

* No spreadsheet format other than CSV, no mapping packs per tool - the following release.
