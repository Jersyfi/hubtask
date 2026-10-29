---
id: UC-DOC-04
title: Link pages and entries and see what links here
context: knowledge
actors: [PE-member, PE-planner, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-18, P-19]
state: built
tasks: []
checked_by: [core/application/service/knowledge/Pages_test.go, test/integration/page_repository_test.go, core/application/service/work/Links_test.go, packages/editor/test/markdown.test.ts]
---

# Link pages and entries and see what links here

## Goal

A page points at entries, views and other pages, and every page shows what points at it - only from what the reader may see.

## Story

The release page mentions the ticket "SUP-128" and embeds the sprint board. The ticket's readers find the page among its backlinks; the "Rollback" page lists the release page as linking to it.

## How to check

1. A page's body that links to `hubtask:<kind>/<id>` mentions the object; an image of one shows it as a card or, for a view, a live view.
2. A reference to a page resolves to its title and icon for a reader of the page's hub, and to its address alone for anybody else, as a missing page does.
3. A page lists the pages, entries' notes and comments that point at it, each with its title, leaving out what the reader may not see.
4. An entry mentioned in a page lists the page among its backlinks.

## Where it ends

* A link between two pages is a reference in the text, not a typed link: typed links stay between entries.
