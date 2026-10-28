---
id: FLW-01
title: Give our work its own types
context: workflow
actors: [PE-admin, PE-member, PE-integrator, PE-agent]
deployments: [D3, D4, D5, D6, D7]
serves: [P-05, P-06, P-08, P-12, P-17]
state: specified
tasks: []
checked_by: []
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
* No type without completion — records are a separate decision (D-07).

## Today

* Check 1: not met — the suite plan, S-03, S-06.
* Check 2: not met — the suite plan, S-03, S-06.
* Check 3: not met — the suite plan, S-03, S-06.
* Check 4: not met — the suite plan, S-03, S-06.
* Check 5: not met — the suite plan, S-03, S-06.
* Check 6: not met — the suite plan, S-03, S-06.
* Check 7: not met — the suite plan, S-03, S-06.
* Check 8: not met — the suite plan, S-03, S-06.
