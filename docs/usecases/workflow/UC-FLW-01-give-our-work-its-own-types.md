---
id: UC-FLW-01
title: Give our work its own types
context: workflow
actors: [PE-admin, PE-member, PE-integrator, PE-agent]
deployments: [D3, D4, D5, D6, D7]
serves: [P-05, P-06, P-08, P-12, P-17]
state: verified
tasks: []
checked_by: [infrastructure/postgres/StructureDocument_test.go, core/application/service/work/StructureInput_test.go, apps/webapp/src/lib/workspace/structureDraft.test.ts, core/domain/model/work/EntryType_test.go, core/application/service/workflow/EntryTypes_test.go, core/application/service/work/SetItemEntryType_test.go, test/integration/entry_type_test.go, apps/webapp/src/lib/data/entrytypes.test.ts, apps/webapp/src/lib/navigation.test.ts, cmd/hubctl/EntryType_test.go, infrastructure/postgres/query/EntryType_test.go, test/integration/query_entry_type_test.go, core/domain/model/work/Form_test.go, core/application/service/work/FormGate_test.go, core/application/service/workflow/EntryTypeForm_test.go, test/integration/entry_type_form_test.go, infrastructure/expression/CEL_test.go, apps/webapp/src/lib/data/entryforms.test.ts, apps/webapp/src/lib/entries/listview.test.ts, core/application/service/work/MoveWorkItem_test.go, core/application/service/work/DuplicateWorkItem_test.go, test/integration/entry_type_placement_test.go, core/domain/model/work/EntryTypeHub_test.go, core/application/service/workflow/EntryTypeHub_test.go, core/application/service/work/EntryTypeHubGate_test.go, test/integration/entry_type_hub_test.go]
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
9. A type may name the types allowed directly underneath it; creating, moving or copying an entry
   of another type under it is refused by name, and the type's editor sets the list.
10. A person who may shape a hub defines types of the hub's own, and makes a copy of a type
    shared from the workspace there under a key of its own, which names the type it was copied
    from; a hub's own type is offered and listed only inside that hub, and an entry or a
    collection carrying it is refused leaving the hub.

## Where it ends

* Types sit on the existing five levels; no new level is created by a type.
* A type's key is unique in the workspace, a hub's own included, so an entry's type names one
  type wherever the entry sits; a copy takes a key of its own. A shared type is never changed
  from a hub (configuration model §2); a collection defines no types of its own.
* A type does not change who may see an entry — that is roles and security levels.
* A type with the *record* behaviour — a configuration item, a risk, a contact — is never
  completed: it has no completion, and completing one is refused.

