---
id: UC-PLN-01
title: Say that one piece of work waits for another
context: planning
actors: [PE-planner, PE-member, PE-admin, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-08, P-18, P-19]
state: partial
tasks: []
checked_by: [core/domain/model/work/Link_test.go, core/domain/model/reference/Reference_test.go, core/application/service/work/Links_test.go, core/application/condition/Links_test.go, core/domain/event/Link_test.go, test/integration/entry_link_test.go]
---

# Say that one piece of work waits for another

## Goal

A planner says that one entry waits for another, or blocks it, duplicates it, runs on it, affects
it or implements it, and everybody who looks at either entry sees the relation from their side -
without ever seeing an entry their role keeps from them.

## Story

The web team plans a relaunch. "Build the pages" depends on "Approve the design", which sits in the
design team's collection. The planner links the two; the design lead opens "Approve the design" and
sees that "Build the pages" waits for it. When somebody tries to make the design wait for the pages
as well, the link is refused: nothing in the loop could ever start. A support agent marks a new
ticket as a duplicate of an older one; a note on the release page mentions the ticket, and the
ticket shows the page among its backlinks. On the timeline, the arrow runs from the design to the
pages.

## How to check

1. A member who may change an entry links it to another entry they can see with one of the
   workspace's link types - blocks, depends on, duplicates, runs on, affects, implements, or one the
   workspace added - and both entries show the link, each from its own side ("blocks" / "is blocked
   by"); whoever may change either end removes it.
2. A link that would close a loop of dependencies - over "depends on" and "blocks" together - is
   refused with `links.cycle`; a type whose cycle rule allows loops, such as "affects", accepts it.
3. An entry duplicates at most one other: a second "duplicates" link from the same entry is refused
   with `links.cardinality`.
4. A link or a reference to an entry the reader may not see shows only "not available to you" -
   never its title, key or state - and nobody can link to an entry they cannot see: it is refused
   as not found, exactly as a missing one is.
5. A person who may shape the workspace adds a link type with its two names, its direction, which
   end waits, whether an entry may start only one, and whether loops are allowed; a type in use can
   only be archived, and an archived type draws no new link while the old ones stay readable.
6. On the timeline, a dependency between two shown entries is drawn from the entry waited for to
   the waiting one, and marked when the waiting entry starts before the other is due.
7. A note or a comment that mentions an entry (a link to `hubtask:entry/<id>`, drawn inline or as a
   card) shows on the mentioned entry as a backlink, for readers who may see the text's entry.
8. REST, MCP and the rule engine draw, read and remove links the same way; a rule's condition reads
   `item.links`; drawing and removing a link announces `item.linked` and `item.unlinked`.

## Where it ends

* No lag or lead on a dependency and no scheduling that moves dates by it - that is the Gantt
  (`PLN-02` in the suite plan's planning module, S-19).
* Links run between entries. References to pages, whiteboards, dashboards, goals and sprints are
  valid and resolve once those modules exist; until then they show "not available".
* A link does not change who may see either entry.

## Today

* Check 1: partly met - every door draws, reads and removes links; the entry's panel in the web
  app does not show them yet (the suite plan, S-05 remainder).
* Check 5: partly met - `GET`/`POST /link-types` and `PATCH`/`DELETE /link-types/{key}`; the
  administration's screen for link types is not built (the suite plan, S-05 remainder).
* Check 6: not met - `POST /links:query` answers a view's dependencies with who waits; the timeline
  does not draw them yet (the suite plan, S-05 remainder).
* Check 7: partly met - the database keeps what notes and comments point at and the entry's links
  answer its backlinks; the web app's editor does not insert mentions or cards yet (the suite plan,
  S-05 remainder, S-14).
