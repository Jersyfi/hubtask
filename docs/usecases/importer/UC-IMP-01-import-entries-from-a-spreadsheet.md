---
id: UC-IMP-01
title: Import entries from a spreadsheet
context: importer
actors: [PE-person, PE-admin, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-08, P-09, P-12]
state: built
tasks: [P-08, F6-09]
checked_by: [core/application/service/importer/Import_test.go, infrastructure/importer/Csv_test.go, test/integration/import_test.go, apps/webapp/src/lib/data/imports.test.ts]
---

# Import entries from a spreadsheet

## Goal

A person who kept their work in a spreadsheet brings it into a hub in one go — titles, notes, due
dates, done or not, labels, columns and parent rows — tells the import which column is which where
the header does not say, and running the same file twice creates nothing twice.

## Story

On a hub the person chooses *Import…*, picks *CSV*, and selects their file. The dialog reads the
header row and offers, for each field, the file's columns — already filled in where a header says
*title*, *due* or *tags*. They point *notes* at the column *Description* and start. A moment later
the report says how many collections, columns, labels and entries were created, and which rows
could not be read and why. A new collection named after the file holds the entries.

## How to check

1. A CSV file with a header row becomes one collection named after the file under the chosen hub —
   or one per value of a `collection` column — with one entry per row.
2. The columns `title`, `notes`, `due`, `completed`, `labels`, `bucket` and `parent` (and their
   usual names, such as *description*, *deadline*, *tags*, *list*) are recognised by their
   header; any field can be pointed at another column, and a column the file does not have is
   refused by name (`imports.mapping_unknown`).
3. Each distinct `bucket` value becomes a board column, each distinct name in `labels` a label, and
   a `parent` names an earlier row by its title or number.
4. A row that cannot be read — no title, an invalid date, an unknown parent — is left out and
   listed in the report with its row number and reason; the other rows land.
5. A date without a time zone is read in the importing person's zone; a file that is not UTF-8 is
   refused with `imports.encoding_invalid`; semicolon-separated files are read as such.
6. Importing the same file into the same hub a second time creates nothing new, and the report says
   the entries were left as they are.
7. A collection the file would create whose name another collection under the hub already has is
   not overwritten; the import stops and reports `imports.collection_exists`.
8. Importing needs the permission to create structure on the hub; the uploaded file must be the
   person's own, uploaded for an import and confirmed.
9. The uploaded file is deleted once the import has run, and the import is in the trail as
   `import.requested`.

## Where it ends

* No export to CSV here; exporting a view is the work context's.
* No custom fields, assignees or comments from a CSV.
* No continuous synchronisation with a spreadsheet; an import is a one-time copy.
* An import is not offered offline.
