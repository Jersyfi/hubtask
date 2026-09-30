---
id: UC-WRK-02
title: Write down a task
context: work
actors: [PE-person, PE-member, PE-child, PE-agent, PE-scripter, PE-integrator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-08, P-10, P-12, P-13]
state: built
tasks: [B-03, B-04, B-05, F2-09, F10-12, F10-17, M-07]
checked_by: [test/integration/create_work_item_test.go, test/integration/work_item_test.go, test/integration/update_test.go, apps/webapp/e2e/entry.test.mjs]
---

# Write down a task

## Goal

Something a person has to do is in the right collection within seconds, with a title and, if
they want, notes — and they can correct either later without opening a form.

## Story

In a collection the person presses *Add*, types a title and presses Enter; the task is at the end
of the list. Opening it shows the title and the notes as text they edit in
place: leaving the field saves, Escape puts the old text back. A family member does the
same on their phone in the groceries list; an agent does it through MCP; a script with `hubctl`.

## How to check

1. In a collection, *Add* opens an inline line with the title; Enter creates the task after the
   last one in the list, and a screen reader announces it by its title.
2. An empty title is refused with `items.title_empty`; a title longer than 500 characters —
   counted in characters, not bytes — with `items.title_too_long`; a title with a line break with
   `items.title_malformed`, which says longer text belongs in the notes.
3. On the entry, the title and the notes are edited in place, with no separate edit mode or form:
   the title saves on Enter or on leaving the field, the notes on leaving the field, and Escape in
   either restores the text as it was.
4. Notes are plain text and are shown exactly as typed.
5. Renaming writes `item.updated` in the history with the old and the new title; changing the
   notes records *that* they changed and none of their text.
6. An entry in an archived collection, or an archived entry, shows its title and notes read-only
   and says why.
7. Creating a task writes `item.created` in its history, and the same request through the API,
   `hubctl` and MCP produces the same entry.

## Where it ends

* No Markdown rendering of notes — deliberately (`apps/webapp/src/lib/people/CommentPanel.svelte`
  says why); a later decision may add it.
* Capturing something that does not yet belong anywhere is the jumble's use case, not this one.
* Creating from mail, a webhook or an import is covered by those contexts.
* What a person with a read-only role sees is [UC-WRK-22](./UC-WRK-22-work-within-what-my-role-allows.md).
