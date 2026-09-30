---
id: UC-IMP-02
title: Move over from Trello, Google Tasks or Microsoft To Do
context: importer
actors: [PE-person, PE-admin, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-09, P-12]
state: built
tasks: [P-09, P-10, F6-09]
checked_by: [infrastructure/importer/Trello_test.go, infrastructure/importer/GoogleTasks_test.go, infrastructure/importer/MicrosoftTodo_test.go, test/integration/import_test.go]
---

# Move over from Trello, Google Tasks or Microsoft To Do

## Goal

Somebody leaving another task app takes their boards and lists with them: the export file that app
produces becomes collections in a hub, with the structure they knew, and without anything they
archived being thrown away.

## Story

The person exports their Trello board as JSON, chooses *Import…* on a hub, picks *Trello* and the
file. The board becomes a collection; its lists become columns; its cards become tasks with their
descriptions, due dates and labels; a card's checklists become the work under it. Cards they had
archived in Trello land archived. The report counts what came across and what did not — the Trello
members, which are not accounts here.

## How to check

1. A Trello board export becomes one collection whose lists are board columns and whose cards are
   tasks with their description as notes, their due date, whether it was met, and their labels
   mapped onto the product's colours.
2. A card's checklists become the work under it — one work package per checklist where there are
   several, an activity per checklist item.
3. A closed card lands archived, not dropped.
4. A card's comments become comments by the importing person, with the original author's name in
   the text.
5. Trello members and their assignments are not carried over, and the report counts them;
   attachments are not fetched.
6. A Google Tasks export (Takeout) and a Microsoft To Do export (the Graph lists with their tasks)
   become one collection per list, with titles, notes, due dates and completion, and a task's
   children under it; a Microsoft To Do reminder becomes a reminder, and its time zone names are
   translated to the standard ones.
7. A file that is not what the chosen kind expects is refused with `imports.file_not_kind`; a kind
   this build does not convert with `imports.kind_unsupported`.
8. Importing the same export twice creates nothing the second time, and the same permission, file
   and trail rules hold as for a spreadsheet import.

## Where it ends

* No live connection to the other app and no import by signing in to it: the person brings the
  export file ([NG-phone-home](../../vision/non-goals.md)).
* No Asana, Jira, Todoist or other sources yet; each is its own converter.
* Attachments stored behind the other app's sign-in are not fetched.
