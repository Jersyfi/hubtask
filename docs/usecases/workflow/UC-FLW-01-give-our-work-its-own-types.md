---
id: UC-FLW-01
title: Give our work its own types
context: workflow
actors: [PE-admin, PE-member, PE-integrator, PE-agent]
deployments: [D3, D4, D5, D6, D7]
serves: [P-05, P-06, P-08, P-12, P-17]
state: partial
tasks: []
checked_by: [core/domain/model/work/EntryType_test.go, core/application/service/workflow/EntryTypes_test.go, core/application/service/work/SetItemEntryType_test.go, test/integration/entry_type_test.go, apps/webapp/src/lib/data/entrytypes.test.ts, apps/webapp/src/lib/navigation.test.ts, cmd/hubctl/EntryType_test.go, infrastructure/postgres/query/EntryType_test.go, test/integration/query_entry_type_test.go, core/domain/model/work/Form_test.go, core/application/service/work/FormGate_test.go, core/application/service/workflow/EntryTypeForm_test.go, test/integration/entry_type_form_test.go, infrastructure/expression/CEL_test.go, apps/webapp/src/lib/data/entryforms.test.ts]
---

# Give our work its own types

## Goal

A team names the kinds of work it does — a bug, a feature, a change request, a purchase — gives each
its icon, its fields and its way of moving, and sees at a glance what kind each entry is.

## Story

The product team of a mid-sized company switches workflows on for its hub. Its administrator adds
"Bug" and "Feature" at the task level and "Sub-task" at the work-package level, gives "Bug" the
fields *Severity* and *Affected version*, and leaves "Feature" with *Estimate*. Members choose the
type when they create an entry; the list and the board show the type's icon; a filter by type
works in every view. An agent creating an entry through MCP names the type the same way.

## How to check

1. A person who may shape the workspace creates, renames, archives and orders types on each level
   (task, work package, activity), each with a name, an icon and a colour from the token set.
2. Each type has its own field set: which custom fields it shows, which are required.
3. Creating an entry offers the types of its level that are allowed in that collection; the
   shipped type of the level is the default.
4. Changing an entry's type keeps every value the new type also has and reports the ones it drops,
   by field, before confirming.
5. A type in use cannot be deleted, only archived; an archived type is not offered for new
   entries and stays readable on the old ones.
6. The type is a filter, a grouping and a column in every view, and a field in the query language.
7. REST, MCP, hubctl and the rule engine create, read and change the type the same way.
8. In a workspace without the workflow module, no type is offered or shown.

## Where it ends

* Types sit on the existing five levels; no new level is created by a type.
* A type does not change who may see an entry — that is roles and security levels.
* A type with the *record* behaviour — a configuration item, a risk, a contact — is never
  completed: it has no completion, and completing one is refused.

## Today

* Check 1: met — `GET`/`POST /entry-types` and `PATCH`/`DELETE /entry-types/{key}` (and the same
  operations over MCP and as rule actions) create, rename, recolour, order and archive types, and
  *How we work → Types* in the administration does the same by level, where the workflows module
  is on.
* Check 2: met — a type has its form (`PUT /entry-types/{key}/form`, the use case
  `SetEntryTypeForm` over REST, MCP and rules): which fields it shows in which section and order,
  which are required, read-only or hidden and when, defaults, validations and channels that only
  narrow it. The server holds every creation, edit and change of type to it, through every door,
  and answers a draft's state (`POST /entry-types/{key}/form:evaluate`). *How we work → Types*
  designs the form; capture unfolds the fields the target type's form requires or may require and
  shows a refusal on its field; the entry panel lays the custom fields out by the form, without
  the hidden ones and with the read-only ones closed. The conditions are evaluated by the server
  only, for the editor as for the write. The archive import writes outside the use case and is
  not held to the form yet (the suite plan, S-23).
* Check 3: partly met — capture offers the workspace's types of the level, the level's own first;
  which types one collection offers is not narrowed yet (the suite plan, S-06).
* Check 4: partly met — a change of type that would drop a value is refused, naming each field,
  and one the entry does not meet the new type's form for is refused the same way;
  confirming the drop and the screen for it come with the field sets (the suite plan, S-06).
* Check 6: partly met — a row marks its type, and the query language filters (`EQ`, `NEQ`, `IN`,
  `NOT_IN`) and groups by `entry_type`, an untyped entry answering as its level's system type; the
  web app's filter, grouping and column are the suite plan's S-11.
* Check 7: met — REST, MCP and the rule engine reach the registry through one descriptor each, and
  `hubctl type ls|create|set|archive|unarchive|rm|assign` and `hubctl item create --entry-type`
  through REST.
