---
id: UC-WRK-44
title: Read and move an object the same way in every product
context: work
actors: [PE-member, PE-guest, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-12, P-17]
state: partial
tasks: []
checked_by: [apps/webapp/src/lib/entries/entrysteps.test.ts, apps/webapp/src/lib/servicedesk/deadlines.test.ts, apps/webapp/e2e/entryhead.test.mjs]
---

# Read and move an object the same way in every product

## Goal

A person who has read one object - a story, a ticket, an inventory object, a goal, a release,
an agent - reads every other one without learning a new screen: the same head, the same place for
the status and the next step, the content in the middle and the properties on the right.

## Story

Amelia opens the story TAMA-18. Across the top: the key with its tile, *Story*, the title, and
under it the status *In progress* as a button, *Request review* as the main button, her name and
the dates. Under the head the jump marks say *Description · Parts 2/8 · Links · Files 9 ·
Conversation 13*; she presses *Files* and lands there. Later she opens the ticket SA-12 from the
desk: the same head, with *Solve* as the main button and one chip saying the resolution deadline
is breached; the deadlines stand at the top of the rail, most urgent first, and *Einordnung*
under them. She writes an internal note on the yellow tab; the requester never sees it. On her
phone the same ticket is one column: the marks are a switch, the properties a sheet.

## How to check

1. A member opens an entry page (story, work package, activity, ticket): one head spans the full
   width with the key, the type, the title, the status button, the next step as the one primary
   button, the person (or *Nobody yet*) and the dates; below it the main column and a property rail.
2. Jump marks under the head name *Description*, *Parts* (done of total where the entry has
   parts), *Links*, *Files* (with the count) and *Conversation* (with the count); pressing one
   brings its section into view.
3. The conversation opens on *All*, which shows the comments and, between them as quiet rows, what
   happened (status changes and other history); *Comments* (on a ticket *Replies* and *Internal
   notes*) and *History* narrow it.
4. On a ticket the reply composer has two tabs, *Reply to …* and *Internal note*; the note tab is
   drawn differently and its button says *Save note*; a note shows in *Internal notes* and not in
   *Replies*.
5. On a ticket the deadlines stand at the top of the rail, most urgent first, and the head carries
   one chip naming what is breached or the next one due; *All deadlines as a table* opens one row
   per policy and one column per goal.
6. The status menu is the one of UC-FLW-06; the step buttons beside the status are gone.
7. On a phone (390 px wide) the jump marks switch between sections, one shown at a time, and the
   properties open as a sheet behind *All properties*.
8. Inventory object, goal, release, agent and portfolio pages start with the same head: key or
   tile, type, title, state and their next act in the same places.

## Where it ends

* What an object carries (fields, sections, which steps exist) is unchanged; this is where it is.
* The administration's pages keep their own layout (page system).

## Today

* Check 3: not proven — the conversation opens on *All* with the history between the comments; no test cites it.
* Check 4: not proven — the composer's *Reply* and *Internal note* tabs are built and the ticket's conversation tabs count replies and notes apart, but no walk saves a note and finds it only under *Internal notes*.
* Check 5: partly met — the rail's deadlines stand first, most urgent first, in the reader's words; the head's one chip and *All deadlines as a table* are not walked.
* Check 6: not proven — the status menu is UC-FLW-06's and the step buttons are gone (UC-FLW-06's tests); no test cites this check.
* Check 8: not proven — inventory object, goal, release, agent and portfolio pages start with the same `ObjectHead`; no walk compares them.
