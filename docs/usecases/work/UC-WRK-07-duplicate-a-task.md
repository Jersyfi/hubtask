---
id: UC-WRK-07
title: Duplicate a task
context: work
actors: [PE-person, PE-member, PE-agent, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-08, P-11, P-12]
state: partial
tasks: [C-11, F3-11]
checked_by: [test/integration/bulk_and_duplicate_test.go, test/integration/copy_test.go, core/application/service/work/DuplicateWorkItem_test.go]
---

# Duplicate a task

## Goal

Something done once and needed again — a packing list, an event checklist — is copied in one
action, with or without everything under it, into the same collection or another one, and the
person is told what the copy could not carry.

## Story

The person opens the row's menu on "Pack for the holidays" and picks *Duplicate*. The dialog
proposes the same title, a switch *with everything under it*, and a destination collection. The
copy arrives open, whatever the original's state; the labels, members and field values that do not
exist or do not apply in the destination are listed by kind.

## How to check

1. *Duplicate* is offered on every entry row — in the list, on the board and in an entry's
   subtree — and opens the same dialog wherever it is chosen.
2. The dialog offers a new title (the original's by default), *with everything under it*, and a
   destination among the collections of the same hub.
3. The copy is a new entry with its own history starting at `item.created`; the original is
   unchanged.
4. With *everything under it*, the whole subtree is copied in the same order; without it, only the
   entry.
5. In another collection, every label, column, custom field value, member and assignee that cannot
   exist there — including a person who cannot see the destination — is left off the copy and
   **listed to the person by kind**.
6. The copy is open and carries no completion, even when the original was done.
7. Duplicating needs the right to create entries in the destination; a person without it is
   refused and nothing is written.

## Where it ends

* No "copy to another hub" from the dialog; the API accepts any collection the person may write
  in.
* Comments and history are not copied — the copy is new work, not a clone of a conversation. What
  travels is the description: title, notes, column, labels, members, assignee, cover, attachments
  (as references to the same files) and custom field values.
* A reusable shape somebody stamps out again and again is a template
  ([UC-WRK-16](./UC-WRK-16-start-a-routine-from-a-template.md)).

## Today

* **Check 1 fails on the entry page.** The row menu of an entry's subtree offers *Duplicate*, but
  the page does not pass the handler to the list, so the choice does nothing
  (`apps/webapp/src/views/ItemView.svelte:825`, `apps/webapp/src/lib/entries/EntryList.svelte:917`).
  From the collection's list and the board it works.
