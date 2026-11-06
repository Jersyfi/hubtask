---
id: UC-DOC-15
title: Prepare changes to a page before readers see them
context: knowledge
actors: [PE-member, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-17]
state: built
tasks: []
checked_by: [core/domain/model/knowledge/Draft_test.go, core/application/service/knowledge/Drafts_test.go, core/application/service/knowledge/AccessDoors_test.go, test/integration/page_repository_test.go, apps/webapp/src/lib/data/pages.test.ts]
---

# Prepare changes to a page before readers see them

## Goal

A space is a live document by default; a hub can switch its space to drafts, so that its writers prepare a page and its changes together and its readers read only what one of them published.

## Story

The HR hub's handbook is read by the whole company. Its team switches the space to drafts. Lena rewrites the parental leave page over two days; colleagues who read the handbook keep seeing the page as it was. When the new rules are agreed, she publishes, and the page changes for everybody at once; its history keeps the published version.

## How to check

1. Somebody who shapes a hub switches its space between the live document and drafts; in a space with drafts a new page is read by its writer alone until it is published.
2. A change to a published page's title or body waits in its draft: the space's writers see the draft and edit it together, its readers read the page as published and cannot open its live document; search, links, export and the offline copy read what was published.
3. A writer publishes a draft - it becomes the page and a version of its history - or throws it away, the page staying as published; a page with nothing waiting is refused (`pages.nothing_to_publish`).
4. The space goes back to the live document only once no draft waits (`pages.drafts_pending`).

## Where it ends

* A page never published is kept to its writer by a restriction naming them; whom else they add reads it too, and publishing lifts that restriction.
* No approval before publishing and no scheduled publishing: whoever may write the page publishes it.
* The workspace decides nothing here; each hub chooses for its own space.
