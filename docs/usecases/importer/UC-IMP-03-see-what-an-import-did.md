---
id: UC-IMP-03
title: See what an import did
context: importer
actors: [PE-person, PE-admin, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-11, P-12]
state: built
tasks: [P-08, F6-09]
checked_by: [core/application/service/importer/Import_test.go, test/integration/import_test.go]
---

# See what an import did

## Goal

After an import the person knows exactly what happened — what was created, what was already there,
what could not be read and why — and every device of every member of the workspace shows the
result without anybody refreshing by hand.

## Story

The import runs in the background; the dialog follows it and then shows the report: *3 collections,
12 columns, 9 labels, 214 entries created; 2 rows not read — row 17: no title, row 88: date not
understood*. `hubctl import` shows the same report. On a colleague's laptop the new collections
appear a moment later.

## How to check

1. Starting an import answers at once with the job and the import it will report on; the import
   is followed until it has succeeded or failed.
2. A finished import reports, per kind of thing, how many were created and how many were left as
   they were, in the same shape a restore reports in.
3. Every row that could not be read is listed with its row number and a reason code, in the
   reader's language.
4. A failed import names its reason; an import stopped halfway resumes where it stopped rather
   than starting over.
5. After an import every device of the workspace loads the workspace's state afresh, so no device
   misses what the import wrote.
6. An import's report is readable by whoever may read the hub it landed in; anybody else is
   refused.

## Where it ends

* No undo of an import as a whole; its collections are deleted like any others.
* No preview of the result before the import runs.
