---
id: UC-WRK-16
title: Start a routine from a template
context: work
actors: [PE-admin, PE-member, PE-person, PE-agent, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-08, P-11, P-12, P-14]
state: built
tasks: [D-06, F3-14, P-11, F6-10]
checked_by: [test/integration/template_test.go, test/integration/template_acceptance_test.go, core/domain/model/work/Template_test.go, core/application/service/work/Template_test.go]
---

# Start a routine from a template

## Goal

A routine a team runs again and again — onboarding a volunteer, preparing a trip, closing the
month — is described once as a tree of steps with dates relative to a start day, and stamped out
into real entries in one action, dated from the day the person picks.

## Story

The club's secretary writes the template "New member": a task with work packages "Paperwork" and
"Welcome", and activities under them, each with a date like *3 days after the start* and, where it
is always the same person, who does it. When a new member joins, any member chooses *From
template*, picks "New member", sets the start day to Monday, and the whole tree appears with its
dates counted from Monday.

## How to check

1. A person who may change the collection's shape creates, edits and deletes a template on the
   collection: a name, a description, and a tree of steps, each with a title, notes, a date
   relative to the start (so many days or weeks before or after, all day or not), a fixed
   responsible person, and steps under it.
2. A tree the collection's capability profile would not allow — a type under a parent it may not
   sit under, a field the type does not carry — is refused when the template is saved, not when
   it is used; a tree of more than 500 steps is refused.
3. A member with the right to create entries uses a template: they pick it, a start day (today by
   default) and optionally a title for the top entry, and every step becomes an entry in the same
   order.
4. Each created entry's due date is the start day plus its step's offset; a step without an offset
   has no date.
5. A fixed responsible person who cannot see the collection is left off the entry, and the person
   using the template is told.
6. Templates defined on the hub or the workspace are offered in every collection below and can be
   used there.
7. A member without the right to change the shape can use templates but is not offered to create,
   edit or delete them.
8. With an AI provider consented to, *Generate from a description* proposes a template that the
   person reviews and saves; without one the action is not shown.

## Where it ends

* No "save this entry as a template"; a template is written, not captured.
* Editing a hub or workspace template happens through the API or `hubctl`; the collection's dialog
  only uses them.
* Instantiated entries do not stay linked to the template: changing the template changes nothing
  already stamped out.
* Repeating on a schedule is [UC-WRK-15](./UC-WRK-15-repeat-a-task.md), not a template.
