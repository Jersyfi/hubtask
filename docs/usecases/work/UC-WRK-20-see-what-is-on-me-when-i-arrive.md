---
id: UC-WRK-20
title: See what is on me when I arrive
context: work
actors: [PE-person, PE-member, PE-child, PE-guest]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-09, P-10, P-12, P-13]
state: partial
tasks: [F10-04]
checked_by: [apps/webapp/e2e/overview.test.mjs]
---

# See what is on me when I arrive

## Goal

The first page after signing in answers "what do I have to do?" — what of mine is overdue, what
is due next, what waits in the inbox, what I had open last — and a brand-new workspace answers it
with the one action that starts it.

## Story

The person opens Hubtask in the morning. *On me* lists two overdue entries at the top and the next
ones by date; the jumble shows it holds four new things; *Recently opened* brings back yesterday's
entry on this laptop. A child sees only their own chores there. In a workspace with no hub yet,
the page holds a single sentence and *Create hub*.

## How to check

1. *On me* lists the open entries the reader is responsible for that have a due date, split into
   *overdue* and *next*, soonest first; where the line falls is decided in the reader's own time
   zone, and an all-day date is overdue only once its day has passed.
2. When more entries are on the reader than the panel shows, a link leads to the search with the
   same narrowing.
3. The jumble panel shows how many entries wait and the first three; it is absent for a person who
   may not read the jumble.
4. *Recently opened* lists what this person opened on **this device**, kept on the device and sent
   nowhere.
5. Nothing on the reader is a sentence saying so, not an empty panel.
6. A workspace with no hub shows one sentence and *Create hub* — to a person who may create one; a
   person whose role reaches no hub is told that nothing has been shared with them yet.
7. Every entry shown is one the reader may read; a child with one hub sees only that hub's work.

## Where it ends

* No dashboard of charts, no team workload, no "what others are doing".
* No panel for entries the reader is only a member of, or that have no due date; those are a
  search or a saved view.
* No configuration of the panels.
* Recently opened does not follow the person to another device (it is deliberately local).

## Today

* Check 3: not met for a person scoped below the workspace — the jumble is read at the workspace, so the read is refused, and the panel is drawn anyway and says nothing is waiting.
* Check 6: not met — the empty state and its *Create hub* are shown to everybody whose list of hubs is empty, also to a member who may not create hubs and to a person who holds no hub.
