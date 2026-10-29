---
id: UC-DOC-01
title: Write a page in a hub and find it again
context: knowledge
actors: [PE-member, PE-person, PE-admin, PE-agent, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-05, P-08, P-19]
state: built
tasks: []
checked_by: [core/domain/model/knowledge/Page_test.go, core/application/service/knowledge/Pages_test.go, test/integration/page_repository_test.go, apps/webapp/src/lib/data/pages.test.ts]
---

# Write a page in a hub and find it again

## Goal

A hub has its pages - a handbook, meeting notes, a specification - beside its work. Whoever may read the hub reads them and finds them again by their words; whoever may not, never learns they exist.

## Story

The web team writes "How we release" in its hub and puts "Checklist" and "Rollback" under it. A month later a new colleague searches for "rollback" and finds the page with the passage that matched. Anna keeps notes in her private hub; nobody else finds them, not even by searching.

## How to check

1. A member who may write in a hub writes a page with a title and a body in its space, at the top or under another page; the space's tree lists it under its parent, in the order its pages were placed.
2. A page moved under itself or under one of its own pages is refused with `pages.parent_cycle`; a page moves only within its own hub's space (`pages.parent_other_space`).
3. A search finds a page by a word of its title or body, stemmed in the page's language, with the passage that matched, among the spaces the reader may read; archived and trashed pages are left out.
4. A page of a hub the reader may not read - a private hub's among them - is neither listed, read nor found; it answers as a missing page does.
5. A page put in the trash goes with the pages under it and comes back with them; nothing is removed.
6. On *Pages* of a hub, a member sees the space's tree, opens a page with its breadcrumb and writes it in place.
7. REST, MCP and `hubctl page …` write, move, read, find and trash pages the same way; the trail records "page changed" and never the words.

## Where it ends

* A page is not an entry: no level, completion, status or assignee.
* No public website and no publishing for the portal here - the service desk's knowledge base comes with M5.
* No import from other wikis here (S-23).
