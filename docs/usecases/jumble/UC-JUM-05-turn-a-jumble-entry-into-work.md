---
id: UC-JUM-05
title: Turn a jumble entry into work
context: jumble
actors: [PE-person, PE-member, PE-scripter, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-05, P-08, P-12]
state: built
tasks: [G-10, F4-12]
checked_by: [core/application/service/jumble/Settlements_test.go, test/integration/jumble_test.go]
---

# Turn a jumble entry into work

## Goal

An arrival in the jumble becomes a task in the collection where it belongs, exactly once, and both
sides remember the connection: the task knows where it came from, the jumble entry knows what it
became.

## Story

The person opens the jumble, picks an undecided entry and presses *Make it an entry*. They choose a
destination from every collection they can see, keep or change the proposed title, and confirm.
The web app takes them straight to the new task. Back in the jumble, the entry now reads *Made into
an entry* with a link to it. Over the API, `hubctl jumble convert` or MCP, the same act can also
choose a board column and make a work package or an activity instead of a task.

## How to check

1. Converting with a destination collection creates one entry there and settles the jumble entry
   as *Made into an entry*, with a link to what it became.
2. Without a title of its own, the new entry takes the jumble entry's subject; without a subject,
   the first line of its body; an entry with neither needs a title and is refused with
   `jumble.title_required`.
3. It becomes a task unless the request names `WORK_PACKAGE` or `ACTIVITY`; a board column is
   honoured when the destination has one.
4. A conversion with no destination is refused with `jumble.destination_required`.
5. The destination's own rights decide: a person who may not create entries in that collection is
   refused as a plain create would be, and the jumble entry stays *Undecided*.
6. A second conversion of the same entry is refused with `jumble.entry_settled`; two conversions
   racing produce one new entry and one refusal, never two entries.
7. The new entry records the jumble entry as its origin, and that origin can never be changed or
   cleared afterwards — not by an edit, not by a device's push.
8. The trail holds `jumble.entry_converted` beside the new entry's own creation.

## Where it ends

* The conversion copies a title and nothing else. The body and the files stay on the jumble entry,
  which the new entry names as its origin; copying them into notes and attachments is not asked for
  here.
* A converted entry is kept, not deleted: it is the new entry's provenance, and the retention sweep
  of never-converted entries does not take it.
* No converting one arrival into several entries; that is a suggestion's decomposition, or several
  creates.
* No undo. A wrong conversion is corrected by moving or deleting the new entry.
