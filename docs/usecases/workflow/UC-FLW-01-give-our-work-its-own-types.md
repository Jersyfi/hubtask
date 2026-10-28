---
id: UC-FLW-01
title: Give our work its own types
context: workflow
actors: [PE-admin, PE-member, PE-integrator, PE-agent]
deployments: [D3, D4, D5, D6, D7]
serves: [P-05, P-06, P-08, P-12, P-17]
state: partial
tasks: []
checked_by: [core/domain/model/work/EntryType_test.go, core/application/service/workflow/EntryTypes_test.go, core/application/service/work/SetItemEntryType_test.go, test/integration/entry_type_test.go, apps/webapp/src/lib/data/entrytypes.test.ts, apps/webapp/src/lib/navigation.test.ts, cmd/hubctl/EntryType_test.go]
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
* Check 2: not met — the suite plan, S-04, S-06.
* Check 3: partly met — capture offers the workspace's types of the level, the level's own first;
  which types one collection offers is not narrowed yet (the suite plan, S-06).
* Check 4: partly met — a change of type that would drop a value is refused, naming each field;
  confirming the drop and the screen for it come with the field sets (the suite plan, S-06).
* Check 6: not met — a row marks its type; the filter, the grouping, the column and the query field
  are the suite plan's S-04 and S-11.
* Check 7: met — REST, MCP and the rule engine reach the registry through one descriptor each, and
  `hubctl type ls|create|set|archive|unarchive|rm|assign` and `hubctl item create --entry-type`
  through REST.
